// Shared playback players for quiz and radio modes

/* ── Audio Player ── */
function AudioPlayer(streamEndpoint) {
    this.streamEndpoint = streamEndpoint;
    this.audioEl = null;
    this.loadTimeout = null;
}

AudioPlayer.prototype.init = function(audioElement) {
    this.audioEl = audioElement;
};

AudioPlayer.prototype.loadAndPlay = function(tuneId, cb) {
    var self = this;
    var settled = false;

    if (self.loadTimeout) clearTimeout(self.loadTimeout);
    self.loadTimeout = setTimeout(function () {
        if (!settled) {
            settled = true;
            self.loadTimeout = null;
            cb(new Error("Audio load timed out"));
        }
    }, 15000);

    fetch(self.streamEndpoint + "/stream?tune_id=" + tuneId)
        .then(function (res) {
            if (!res.ok) throw new Error("stream " + res.status);
            return res.json();
        })
        .then(function (data) {
            if (settled) return;
            settled = true;
            if (self.loadTimeout) { clearTimeout(self.loadTimeout); self.loadTimeout = null; }
            
            self.audioEl.src = data.url;
            self.audioEl.load();
            self.audioEl.play().catch(function () {
                // Autoplay blocked by browser, silent fail
            });
            cb();
        })
        .catch(function (err) {
            if (!settled) {
                settled = true;
                if (self.loadTimeout) { clearTimeout(self.loadTimeout); self.loadTimeout = null; }
                cb(new Error("Audio unavailable"));
            }
        });
};

AudioPlayer.prototype.pause = function () {
    if (this.audioEl) this.audioEl.pause();
};

AudioPlayer.prototype.getCurrentTime = function () {
    return this.audioEl ? this.audioEl.currentTime : 0;
};

AudioPlayer.prototype.getDuration = function () {
    return this.audioEl ? this.audioEl.duration : 0;
};

/* ── YouTube Player ── */
function YoutubePlayer() {
    this.ytPlayer = null;
    this.loadTimeout = null;
    this.onVideoReady = null;
    this.onLoadError = null;
}

YoutubePlayer.prototype.init = function(containerId, cb) {
    var self = this;
    self.ytPlayer = new YT.Player(containerId, {
        height: "180",
        width: "100%",
        playerVars: { autoplay: 0, controls: 1, enablejsapi: 1, modestbranding: 1, rel: 0 },
        events: {
            onReady: function () { if (cb) cb(); },
            onStateChange: function (ev) {
                if (ev.data === YT.PlayerState.PLAYING && self.onVideoReady) {
                    self.onVideoReady();
                    self.onVideoReady = null;
                }
            },
            onError: function () {
                if (self.onLoadError) {
                    var fail = self.onLoadError;
                    self.onLoadError = null;
                    self.onVideoReady = null;
                    if (self.loadTimeout) { clearTimeout(self.loadTimeout); self.loadTimeout = null; }
                    fail(new Error("Video unavailable"));
                }
            }
        }
    });
};

YoutubePlayer.prototype.loadAndPlay = function (videoId, cb, opts) {
    var self = this;
    var settled = false;
    opts = opts || {};
    var minDuration = opts.minDuration || 25;
    var randomSeek = opts.randomSeek !== false; // default true

    self.onLoadError = cb;
    
    if (self.loadTimeout) clearTimeout(self.loadTimeout);
    self.loadTimeout = setTimeout(function () {
        if (!settled && self.onLoadError) {
            settled = true;
            self.onVideoReady = null;
            self.onLoadError = null;
            self.loadTimeout = null;
            cb(new Error("Video load timed out"));
        }
    }, 15000);

    self.onVideoReady = function () {
        if (settled) return;
        settled = true;
        if (self.loadTimeout) { clearTimeout(self.loadTimeout); self.loadTimeout = null; }
        self.onVideoReady = null;
        self.onLoadError = null;
        var dur = self.ytPlayer.getDuration();
        if (dur < minDuration) {
            self.ytPlayer.stopVideo();
            cb(new Error("Video too short"));
            return;
        }
        if (randomSeek) {
            var start = Math.floor(dur * (0.10 + Math.random() * 0.70));
            self.ytPlayer.seekTo(start);
        }
        cb();
    };
    self.ytPlayer.loadVideoById(videoId);
};

YoutubePlayer.prototype.pause = function () {
    if (this.ytPlayer && typeof this.ytPlayer.pauseVideo === "function") {
        this.ytPlayer.pauseVideo();
    }
};

YoutubePlayer.prototype.play = function () {
    if (this.ytPlayer && typeof this.ytPlayer.playVideo === "function") {
        this.ytPlayer.playVideo();
    }
};

YoutubePlayer.prototype.getCurrentTime = function () {
    return this.ytPlayer ? this.ytPlayer.getCurrentTime() : 0;
};

YoutubePlayer.prototype.getDuration = function () {
    return this.ytPlayer ? this.ytPlayer.getDuration() : 0;
};
