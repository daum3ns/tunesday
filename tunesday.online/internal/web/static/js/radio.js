(function () {
    "use strict";

    var script = document.currentScript;
    var wsPath = script.getAttribute("data-ws");
    var basePath = script.getAttribute("data-base");

    var statusEl = document.getElementById("radio-status");
    var trackEl = document.getElementById("radio-track");
    var joinBtn = document.getElementById("join-btn");
    var posEl = document.getElementById("radio-pos");
    var listenersEl = document.getElementById("radio-listeners");
    var queueInfo = document.getElementById("queue-info");

    var btnPlay = document.getElementById("btn-play");
    var btnPause = document.getElementById("btn-pause");
    var btnPrev = document.getElementById("btn-prev");
    var btnNext = document.getElementById("btn-next");
    var btnShuffle = document.getElementById("btn-shuffle");
    var btnStream = document.getElementById("btn-stream");

    var audioEl = document.getElementById("radio-audio");
    var noteEl = document.getElementById("radio-stream-note");
    var eqCanvas = document.getElementById("radio-eq");

    var ws = null;
    var state = null;
    var arrivedAt = 0;

    var player = null;          // YT.Player — lazy: created only for iframe mode
    var iframeLoading = false;
    var joined = false;         // user gesture happened (audio may start)
    var currentTuneId = 0;      // tune cued in the iframe
    var endedSentFor = 0;
    var joinTimeout = null;

    var streamOn = (function () {
        try { return localStorage.getItem("tunesday-stream") === "1"; } catch (e) { return false; }
    })();
    var audioActive = false;
    var audioPending = false;   // stream src set, canplay not yet reached
    var audioTuneId = 0;
    var watchdog = null;
    var streamStrikes = 0;      // consecutive failures; trips the fallback

    /* ── media abstraction: audio element takes over in stream mode ── */

    function iframeMuted() { return audioActive || audioPending; }

    function setIframeVolume() {
        if (!player || typeof player.setVolume !== "function") return;
        try { player.setVolume(iframeMuted() ? 0 : 100); } catch (e) {}
    }

    function mediaTime() {
        if (audioActive && audioEl) { try { return audioEl.currentTime || 0; } catch (e) { return 0; } }
        if (player && player.getCurrentTime) { try { return player.getCurrentTime() || 0; } catch (e) { return 0; } }
        return 0;
    }

    function mediaDuration() {
        var d = 0;
        if (audioActive && audioEl) d = audioEl.duration;
        else if (player && player.getDuration) d = player.getDuration();
        if (!d || !isFinite(d)) return 0;
        return d;
    }

    function mediaSeek(p) {
        if (audioActive && audioEl) { try { audioEl.currentTime = p; } catch (e) {} return; }
        if (player && player.seekTo) player.seekTo(p, true);
    }

    function pauseIframe() {
        if (player && typeof player.pauseVideo === "function") player.pauseVideo();
    }

    /* ── transport ── */

    function connect() {
        var proto = location.protocol === "https:" ? "wss://" : "ws://";
        ws = new WebSocket(proto + location.host + wsPath);
        ws.onmessage = function (ev) {
            var msg = JSON.parse(ev.data);
            if (msg.type === "radio_state") applyState(msg.payload);
        };
        ws.onclose = function () {
            statusEl.textContent = "broadcast lost… reconnecting";
            setTimeout(connect, 2000);
        };
    }

    function position() {
        if (!state || !state.tune) return 0;
        if (state.status === "paused") return state.elapsedSec || 0;
        return (state.elapsedSec || 0) + (Date.now() - arrivedAt) / 1000;
    }

    function applyState(p) {
        state = p;
        arrivedAt = Date.now();
        renderListeners(p.listeners || []);
        renderControls();

        if (!p.tune) {
            statusEl.textContent = "the decks are cold — press play to start the party";
            trackEl.textContent = "nothing playing";
            posEl.textContent = "";
            releaseAudio();
            if (player) player.stopVideo();
            currentTuneId = 0;
            return;
        }

        var prefix = joined ? "🔊 " : "";
        statusEl.textContent = prefix + (p.status === "paused"
            ? "PAUSED — someone stepped away from the mixer"
            : "▶ live");
        trackEl.textContent = "♪ " + p.tune.title + "  [ " + p.tune.provider + " ]";

        if (!joined) {
            joinBtn.hidden = p.status !== "playing";
            return;
        }

        if (streamOn && audioEl) syncAudio();
        // While the stream is loading or playing, the iframe must stay silent.
        if (!audioActive && !audioPending) cueAndSync();
    }

    /* ── iframe player (lazy: only for iframe mode or as fallback) ── */

    function ensureIframe() {
        if (player || iframeLoading) return;
        iframeLoading = true;
        var go = function () {
            new YT.Player("radio-player", {
                width: "100%",
                height: "240",
                playerVars: { playsinline: 1, rel: 0 },
                events: {
                    onReady: function () {
                        iframeLoading = false;
                        playerReady();
                    },
                    onStateChange: function () { /* sync is position()-driven */ }
                }
            });
        };
        if (window.YT && YT.Player) { go(); return; }
        window.onYouTubeIframeAPIReady = go;
        var s = document.createElement("script");
        s.src = "https://www.youtube.com/iframe_api";
        document.head.appendChild(s);
    }

    function playerReady() {
        if (!joined) markJoined();
        if (state) applyState(state);
    }

    function cueAndSync() {
        if (!player || !player.cueVideoById || !state || !state.tune) return;
        var t = state.tune;
        if (currentTuneId !== t.id) {
            currentTuneId = t.id;
            player.cueVideoById(t.youtubeId);
        }
        if (state.status === "playing") {
            player.playVideo();
            var drift = Math.abs((player.getCurrentTime() || 0) - position());
            if (drift > 3) player.seekTo(position(), true);
        } else if (state.status === "paused") {
            player.pauseVideo();
        }
        setIframeVolume();
    }

    /* ── direct stream (audio element) ── */

    function note(msg) {
        if (!noteEl) return;
        if (!msg) { noteEl.hidden = true; noteEl.textContent = ""; return; }
        noteEl.hidden = false;
        noteEl.textContent = msg;
    }

    function updateStreamBtn() {
        if (!btnStream) return;
        btnStream.textContent = "⚡ stream: " + (streamOn ? "on" : "off");
        btnStream.classList.toggle("active", streamOn);
    }

    function syncAudio() {
        var t = state.tune;
        if (audioTuneId !== t.id) {
            releaseAudio();
            audioTuneId = t.id;
            audioPending = true;
            audioEl.src = basePath + "/stream?tune_id=" + t.id;
            try { audioEl.load(); } catch (e) { return streamFail("browser blocked the stream"); }
            armWatchdog();
        }
        setIframeVolume();
        if (state.status === "playing") {
            tryPlayAudio();
            pauseIframe();
        } else if (state.status === "paused") {
            if (audioActive) {
                audioEl.pause();
                try { audioEl.currentTime = position(); } catch (e) {}
            }
        }
    }

    function tryPlayAudio() {
        if (!audioActive) return; // becomes live on 'canplay'
        var p = position();
        try { audioEl.currentTime = p; } catch (e) {}
        audioEl.play().catch(function () {
            setTimeout(function () {
                if (audioActive && state && state.status === "playing") {
                    audioEl.play().catch(function () { armWatchdog(); });
                }
            }, 400);
        });
    }

    function armWatchdog() {
        if (watchdog) clearTimeout(watchdog);
        watchdog = setTimeout(function () {
            if (!audioActive && streamOn && audioTuneId !== 0) streamFail("the stream stayed silent");
        }, 5000);
    }

    function streamFail(reason) {
        if (watchdog) { clearTimeout(watchdog); watchdog = null; }
        if (!audioActive && audioTuneId === 0) return;
        releaseAudio();
        streamStrikes++;
        if (streamStrikes >= 3 && streamOn) {
            // circuit breaker: stop retry-looping this session
            streamOn = false;
            updateStreamBtn();
            note("⚠ the stream kept failing (" + reason + ") — using the YouTube player for this session");
        } else {
            note("⚠ direct stream hiccuped (" + reason + ") — retrying via the YouTube player");
        }
        ensureIframe(); // lazy rescue
        if (state) applyState(state);
    }

    function releaseAudio() {
        if (watchdog) { clearTimeout(watchdog); watchdog = null; }
        var was = audioActive;
        audioActive = false;
        audioPending = false;
        audioTuneId = 0;
        if (audioEl) {
            try { audioEl.pause(); } catch (e) {}
            audioEl.removeAttribute("src");
            try { audioEl.load(); } catch (e2) {}
        }
        if (was) stopEq();
        setIframeVolume();
    }

    if (audioEl) {
        audioEl.addEventListener("canplay", function () {
            if (!streamOn) return;
            if (audioTuneId === 0 || !state || !state.tune || audioTuneId !== state.tune.id) return;
            audioActive = true;
            audioPending = false;
            streamStrikes = 0;
            if (watchdog) { clearTimeout(watchdog); watchdog = null; }
            note("");
            setIframeVolume();
            pauseIframe();
            tryPlayAudio();
            startEq();
        });
        audioEl.addEventListener("error", function () {
            if (audioTuneId !== 0) streamFail("the server could not deliver the audio");
        });
        audioEl.addEventListener("stalled", function () { armWatchdog(); });
        audioEl.addEventListener("ended", function () {
            if (audioActive && state && state.tune && endedSentFor !== state.tune.id) {
                endedSentFor = state.tune.id;
                command("ended", { tune_id: state.tune.id });
            }
        });
    }

    if (btnStream) btnStream.addEventListener("click", function () {
        streamOn = !streamOn;
        try { localStorage.setItem("tunesday-stream", streamOn ? "1" : "0"); } catch (e) {}
        updateStreamBtn();
        if (!streamOn) {
            releaseAudio();
            note("");
            ensureIframe(); // user chose the iframe player: boot it now
            if (state) applyState(state);
            return;
        }
        ensureAudioGraph();
        note("trying the direct stream…");
        if (state) applyState(state);
    });

    /* ── equalizer (works only for same-origin stream audio) ── */

    var audioCtx = null, analyser = null, eqRunning = false, eqRaf = 0;

    function ensureAudioGraph() {
        if (!streamOn || analyser || !audioEl) return;
        var AC = window.AudioContext || window.webkitAudioContext;
        if (!AC || !eqCanvas) return;
        try {
            audioCtx = new AC();
            if (audioCtx.state === "suspended") audioCtx.resume();
            var src = audioCtx.createMediaElementSource(audioEl);
            analyser = audioCtx.createAnalyser();
            analyser.fftSize = 128;
            src.connect(analyser);
            analyser.connect(audioCtx.destination);
        } catch (e) {
            // createMediaElementSource reroutes audio through the graph; a broken
            // graph would render the element silent, so abandon stream mode.
            analyser = null;
            streamOn = false;
            try { localStorage.setItem("tunesday-stream", "0"); } catch (e2) {}
            updateStreamBtn();
            releaseAudio();
            note("this browser cannot build the audio graph — using the YouTube player");
            ensureIframe();
        }
    }

    function startEq() {
        if (!analyser || !eqCanvas || eqRunning) return;
        eqRunning = true;
        eqCanvas.hidden = false;
        var c2d = eqCanvas.getContext("2d");
        var data = new Uint8Array(analyser.frequencyBinCount);
        function frame() {
            if (!eqRunning) return;
            analyser.getByteFrequencyData(data);
            var w = eqCanvas.width, h = eqCanvas.height;
            c2d.clearRect(0, 0, w, h);
            c2d.fillStyle = "#0f0";
            var bars = 32, bw = w / bars;
            for (var i = 0; i < bars; i++) {
                var v = data[i * 2] / 255;
                var bh = Math.max(2, v * (h - 2));
                c2d.fillRect(i * bw + 1, h - bh, bw - 2, bh);
            }
            eqRaf = requestAnimationFrame(frame);
        }
        frame();
        if (audioCtx && audioCtx.state === "suspended") audioCtx.resume();
    }

    function stopEq() {
        eqRunning = false;
        if (eqRaf) cancelAnimationFrame(eqRaf);
        if (eqCanvas) eqCanvas.hidden = true;
    }

    /* ── controls ── */

    function markJoined() {
        joined = true;
        joinBtn.hidden = true;
        if (joinTimeout) { clearTimeout(joinTimeout); joinTimeout = null; }
        if (state) {
            statusEl.textContent = "🔊 you're in — you'll hear the room from now on";
        }
    }

    joinBtn.addEventListener("click", function () {
        if (streamOn) {
            // stream mode: no YouTube API at all until something breaks
            ensureAudioGraph();
            if (state && state.tune) {
                markJoined();
                syncAudio();
            } else {
                markJoined();
            }
            return;
        }
        ensureIframe();
        joinTimeout = setTimeout(function () {
            if (!joined) {
                note("⚠ the YouTube player did not load (adblock?) — try ⚡ stream mode");
                joinBtn.hidden = false;
            }
        }, 6000);
    });

    function command(path, fields) {
        var body = new URLSearchParams(fields || {}).toString();
        fetch(basePath + "/" + path, {
            method: "POST",
            headers: { "Content-Type": "application/x-www-form-urlencoded" },
            body: body
        });
    }

    btnPlay.addEventListener("click", function () { command("play"); });
    btnPause.addEventListener("click", function () { command("pause"); });
    btnPrev.addEventListener("click", function () { command("prev"); });
    btnNext.addEventListener("click", function () { command("next"); });
    btnShuffle.addEventListener("click", function () {
        var mode = state && state.mode === "shuffled" ? "ordered" : "shuffled";
        command("mode", { mode: mode });
    });

    Array.prototype.forEach.call(document.querySelectorAll(".playlist-row .play-this"), function (btn) {
        btn.addEventListener("click", function () {
            command("play", { tune_id: btn.closest("tr").dataset.tune });
        });
    });

    function renderListeners(list) {
        listenersEl.innerHTML = "";
        if (!list.length) {
            listenersEl.innerHTML = '<li class="muted">nobody connected</li>';
            return;
        }
        list.forEach(function (l) {
            var li = document.createElement("li");
            li.className = l.live ? "" : "muted";
            li.textContent = (l.live ? "⏻" : "○") + " " + l.alias
                + (l.provider ? "  [ " + l.provider + " ]" : "")
                + (l.isYou ? "  ← you" : "")
                + (l.live ? "" : "  (left)");
            listenersEl.appendChild(li);
        });
    }

    function renderControls() {
        var hasTunes = state && state.queueLen > 0;
        var playing = state && state.status === "playing";
        btnPlay.disabled = !hasTunes || playing;
        btnPause.disabled = !playing;
        btnPrev.disabled = !hasTunes;
        btnNext.disabled = !hasTunes;
        btnShuffle.textContent = "⇄ mode: " + (state && state.mode === "shuffled" ? "shuffled" : "ordered");
        if (state) {
            queueInfo.textContent = state.queueLen ? "(" + (state.index + 1) + " / " + state.queueLen + " in cycle)" : "";
        }
    }

    // ticker + gentle drift correction + iframe track-end reporting
    setInterval(function () {
        if (!state || !state.tune) return;
        var pos = position();
        var dur = mediaDuration();
        var live = mediaTime();
        posEl.textContent = fmt(state.status === "paused" ? pos : Math.max(live, pos))
            + (dur > 1 ? " / " + fmt(dur) : "")
            + (joined || audioActive ? "" : "  (press join to hear it)");

        if (!joined && !audioActive) return;
        if (state.status !== "playing") return;

        var drift = pos - live;
        if (Math.abs(drift) > 3) {
            mediaSeek(pos);
            if (audioEl) audioEl.playbackRate = 1;
        } else if (audioActive && audioEl && Math.abs(drift) > 0.4) {
            // inaudible catch-up: run slightly fast/slow instead of jumping
            audioEl.playbackRate = drift > 0 ? 1.04 : 0.96;
        } else if (audioActive && audioEl) {
            audioEl.playbackRate = 1;
        }

        if (!(audioActive && audioEl) && dur > 1 && live >= dur - 1 && endedSentFor !== state.tune.id) {
            endedSentFor = state.tune.id;
            command("ended", { tune_id: state.tune.id });
        }
    }, 1000);

    function fmt(s) {
        s = Math.max(0, Math.floor(s));
        return Math.floor(s / 60) + ":" + ("0" + (s % 60)).slice(-2);
    }

    updateStreamBtn();
    connect();
})();
