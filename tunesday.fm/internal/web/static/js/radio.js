(function () {
    "use strict";

    var script = document.currentScript;
    var wsPath = script.getAttribute("data-ws");
    var basePath = script.getAttribute("data-base");

    var statusEl = document.getElementById("radio-status");
    var trackEl = document.getElementById("radio-track");
    var playerBox = document.getElementById("radio-player");
    var joinBtn = document.getElementById("join-btn");
    var posEl = document.getElementById("radio-pos");
    var listenersEl = document.getElementById("radio-listeners");
    var queueInfo = document.getElementById("queue-info");

    var btnPlay = document.getElementById("btn-play");
    var btnPause = document.getElementById("btn-pause");
    var btnPrev = document.getElementById("btn-prev");
    var btnNext = document.getElementById("btn-next");
    var btnShuffle = document.getElementById("btn-shuffle");

    var ws = null;
    var player = null;          // YT.Player
    var joined = false;         // user gesture happened, player exists
    var state = null;           // last radio_state payload
    var arrivedAt = 0;          // Date.now() when state arrived
    var currentTuneId = 0;      // tune the player has cued
    var endedSentFor = 0;       // tune we already reported finished
    var pendingPlay = false;

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
        var drift = (Date.now() - arrivedAt) / 1000;
        return (state.elapsedSec || 0) + drift;
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
            if (player) player.stopVideo();
            return;
        }

        statusEl.textContent = p.status === "paused" ? "PAUSED — someone stepped away from the mixer" : "▶ live";
        trackEl.textContent = "♪ " + p.tune.title + "  [ " + p.tune.provider + " ]";

        if (!joined) {
            joinBtn.hidden = p.status !== "playing";
            return;
        }

        cueAndSync();
    }

    function cueAndSync() {
        if (!player || !player.cueVideoById) return;
        var t = state.tune;
        if (currentTuneId !== t.id) {
            currentTuneId = t.id;
            endedSentFor = 0;
            player.cueVideoById(t.youtubeId);
        }
        if (state.status === "playing") {
            player.seekTo(position(), true);
            if (pendingPlay) return; // wait for READY below
            var st = player.getPlayerState && player.getPlayerState();
            if (st === -1) { pendingPlay = true; }
            else player.playVideo();
        } else if (state.status === "paused") {
            player.seekTo(position(), true);
            player.pauseVideo();
        }
    }

    function onReady() {
        joined = true;
        joinBtn.hidden = true;
        pendingPlay = false;
        if (state && state.status === "playing" && state.tune) {
            currentTuneId = 0; // force cue
            cueAndSync();
            if (player) player.playVideo();
        }
    }

    function loadYouTubeAPI(cb) {
        if (window.YT && YT.Player) return cb();
        window.onYouTubeIframeAPIReady = function () { cb(); };
        var s = document.createElement("script");
        s.src = "https://www.youtube.com/iframe_api";
        document.head.appendChild(s);
    }

    joinBtn.addEventListener("click", function () {
        loadYouTubeAPI(function () {
            player = new YT.Player("radio-player", {
                width: "100%",
                height: "315",
                playerVars: { playsinline: 1, rel: 0 },
                events: {
                    onReady: onReady,
                    onStateChange: function (ev) {
                        if (ev.data === YT.PlayerState.PLAYING && pendingPlay) {
                            pendingPlay = false;
                            player.seekTo(position(), true);
                        }
                    }
                }
            });
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
            li.textContent = "⏻ " + l.alias + (l.provider ? "  [ " + l.provider + " ]" : "") + (l.isYou ? "  ← you" : "");
            listenersEl.appendChild(li);
        });
    }

    function renderControls() {
        var hasTunes = state && state.queueLen > 0;
        var playing = state && state.status === "playing";
        var paused = state && state.status === "paused";
        btnPlay.disabled = !hasTunes || playing;
        btnPause.disabled = !playing;
        btnPrev.disabled = !hasTunes;
        btnNext.disabled = !hasTunes;
        btnShuffle.textContent = "⇄ mode: " + (state && state.mode === "shuffled" ? "shuffled" : "ordered");
        if (state) {
            queueInfo.textContent = state.queueLen ? "(" + state.index + " / " + state.queueLen + " in cycle)" : "";
        }
    }

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
    btnNext.addEventListener("click", function () { command("next"); });
    btnPrev.addEventListener("click", function () { command("prev"); });
    btnShuffle.addEventListener("click", function () {
        var mode = state && state.mode === "shuffled" ? "ordered" : "shuffled";
        command("mode", { mode: mode });
    });

    Array.prototype.forEach.call(document.querySelectorAll(".playlist-row .play-this"), function (btn) {
        btn.addEventListener("click", function () {
            command("play", { tune_id: btn.closest("tr").dataset.tune });
        });
    });

    // Position ticker, drift correction, and the "ended" reporter.
    setInterval(function () {
        if (!state || !state.tune) return;
        var pos = position();
        var dur = joined && player && player.getDuration ? player.getDuration() : 0;
        posEl.textContent = state.status === "idle" ? "" :
            fmt(pos) + (dur > 1 ? " / " + fmt(dur) : "") + (joined ? "" : "  (join to hear it)");

        if (joined && player && state.status === "playing") {
            var cur = player.getCurrentTime();
            if (Math.abs(cur - pos) > 1.5) player.seekTo(pos, true);
            if (dur > 1 && cur >= dur - 1 && endedSentFor !== state.tune.id) {
                endedSentFor = state.tune.id;
                command("ended", { tune_id: state.tune.id });
            }
        }
    }, 1000);

    function fmt(s) {
        s = Math.max(0, Math.floor(s));
        return Math.floor(s / 60) + ":" + ("0" + (s % 60)).slice(-2);
    }

    connect();
})();
