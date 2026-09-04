(function () {
    "use strict";

    var script = document.currentScript;
    var wsPath = script.getAttribute("data-ws");
    var basePath = script.getAttribute("data-base");

    var statusEl  = document.getElementById("radio-status");
    var trackEl   = document.getElementById("radio-track");
    var posEl     = document.getElementById("radio-pos");
    var progressFill = document.getElementById("radio-progress-fill");
    var audioEl   = document.getElementById("radio-audio");
    var listenersEl = document.getElementById("radio-listeners");
    var btnPlay   = document.getElementById("btn-play");
    var btnPause  = document.getElementById("btn-pause");
    var btnNext   = document.getElementById("btn-next");
    var btnPrev   = document.getElementById("btn-prev");
    var btnShuffle = document.getElementById("btn-shuffle");

    // Collect tunes from the playlist table in the DOM.
    var tunes = [];
    var rows = document.querySelectorAll(".playlist-row, tr[data-tune]");
    for (var i = 0; i < rows.length; i++) {
        var row = rows[i];
        var id = parseInt(row.getAttribute("data-tune"), 10);
        if (id) tunes.push(id);
    }

    var queue = [];
    var queueIndex = 0;
    var currentMode = "ordered";
    var currentTuneId = 0;
    var ws = null;
    var posTimer = null;

    // ── Queue management (client-side) ──

    function buildQueue(shuffle) {
        queue = tunes.slice();
        if (shuffle) {
            for (var i = queue.length - 1; i > 0; i--) {
                var j = Math.floor(Math.random() * (i + 1));
                var tmp = queue[i]; queue[i] = queue[j]; queue[j] = tmp;
            }
        }
        queueIndex = 0;
    }

    function playTune(tuneId) {
        var idx = queue.indexOf(tuneId);
        if (idx === -1) {
            queue.unshift(tuneId);
            idx = 0;
        }
        queueIndex = idx;
        loadAndPlay(tuneId);
    }

    function nextTune() {
        if (queue.length === 0) return;
        queueIndex++;
        if (queueIndex >= queue.length) {
            buildQueue(currentMode === "shuffled");
        }
        loadAndPlay(queue[queueIndex]);
    }

    function prevTune() {
        if (queue.length === 0) return;
        queueIndex--;
        if (queueIndex < 0) {
            buildQueue(currentMode === "shuffled");
            queueIndex = queue.length - 1;
        }
        loadAndPlay(queue[queueIndex]);
    }

    function toggleMode() {
        currentMode = currentMode === "shuffled" ? "ordered" : "shuffled";
        var currentId = queue[queueIndex];
        buildQueue(currentMode === "shuffled");
        if (currentId) {
            var idx = queue.indexOf(currentId);
            if (idx !== -1) queueIndex = idx;
        }
        btnShuffle.textContent = "⇄ " + currentMode;
    }

    // ── Audio ──

    function loadAndPlay(tuneId) {
        if (!tuneId) return;
        currentTuneId = tuneId;
        resetProgress();
        statusEl.textContent = "loading…";
        trackEl.textContent = "♪ " + tuneTitle(tuneId);

        fetch(basePath + "/stream?tune_id=" + tuneId)
            .then(function (res) {
                if (!res.ok) throw new Error("stream " + res.status);
                return res.json();
            })
            .then(function (data) {
                if (currentTuneId !== tuneId) return;
                audioEl.src = data.url;
                audioEl.load();
                audioEl.play().then(function () {
                    statusEl.textContent = "▶ live";
                    trackEl.textContent = "♪ " + tuneTitle(tuneId);
                    reportNowPlaying(tuneId);
                    startPosTimer();
                }).catch(function () {
                    statusEl.textContent = "▶ live (click play to unmute)";
                });
            })
            .catch(function () {
                statusEl.textContent = "⚠ stream unavailable";
            });
    }

    function tuneTitle(id) {
        // Fallback: scan the table rows for the title text.
        var rows = document.querySelectorAll("tr[data-tune]");
        for (var i = 0; i < rows.length; i++) {
            if (parseInt(rows[i].getAttribute("data-tune"), 10) === id) {
                var cells = rows[i].querySelectorAll("td");
                return cells.length > 1 ? cells[1].textContent : "unknown";
            }
        }
        return "unknown";
    }

    function startPosTimer() {
        if (posTimer) clearInterval(posTimer);
        posTimer = setInterval(function () {
            if (audioEl.paused || !audioEl.duration) { posEl.textContent = ""; progressFill.style.width = "0%"; return; }
            posEl.textContent = fmt(audioEl.currentTime) + " / " + fmt(audioEl.duration);
            progressFill.style.width = (audioEl.currentTime / audioEl.duration * 100) + "%";
        }, 250);
    }

    function resetProgress() {
        posEl.textContent = "";
        progressFill.style.width = "0%";
    }

    function fmt(s) {
        s = Math.max(0, Math.floor(s));
        return Math.floor(s / 60) + ":" + ("0" + (s % 60)).slice(-2);
    }

    // ── WebSocket (presence) ──

    function connect() {
        var proto = location.protocol === "https:" ? "wss://" : "ws://";
        ws = new WebSocket(proto + location.host + wsPath);
        ws.onmessage = function (ev) {
            var msg = JSON.parse(ev.data);
            if (msg.type === "radio_listeners") renderListeners(msg.payload);
        };
        ws.onclose = function () {
            statusEl.textContent = "disconnected — reconnecting…";
            setTimeout(connect, 2000);
        };
    }

    function reportNowPlaying(tuneId) {
        if (ws && ws.readyState === 1) {
            ws.send(JSON.stringify({ type: "now_playing", tune_id: tuneId }));
        }
    }

    function renderListeners(list) {
        listenersEl.innerHTML = "";
        if (!list || !list.length) {
            listenersEl.innerHTML = '<li class="muted">nobody connected</li>';
            return;
        }
        for (var i = 0; i < list.length; i++) {
            var l = list[i];
            var li = document.createElement("li");
            li.className = "";
            var text = l.alias;
            if (l.provider) text += "  [ " + l.provider + " ]";
            if (l.isYou) text += "  ← you";
            if (l.tuneId) text += "  ♪ " + l.tuneTitle;
            li.textContent = text;
            listenersEl.appendChild(li);
        }
    }

    // ── Transport controls ──

    btnPlay.addEventListener("click", function () {
        if (currentTuneId && audioEl.src) {
            audioEl.play().then(function () {
                statusEl.textContent = "▶ live";
                startPosTimer();
            }).catch(function () {});
            return;
        }
        if (queue.length === 0) buildQueue(false);
        loadAndPlay(queue[0]);
    });

    btnPause.addEventListener("click", function () {
        audioEl.pause();
        statusEl.textContent = "❚❚ paused";
        if (posTimer) { clearInterval(posTimer); posTimer = null; }
        resetProgress();
    });

    btnNext.addEventListener("click", nextTune);
    btnPrev.addEventListener("click", prevTune);
    btnShuffle.addEventListener("click", toggleMode);

    // Playlist row clicks.
    var playBtns = document.querySelectorAll(".play-this");
    for (var i = 0; i < playBtns.length; i++) {
        playBtns[i].addEventListener("click", function () {
            var id = parseInt(this.closest("tr").getAttribute("data-tune"), 10);
            if (id) playTune(id);
        });
    }

    // ── Audio events ──

    audioEl.addEventListener("play", function () {
        statusEl.textContent = "▶ live";
        btnPlay.hidden = true;
        btnPause.hidden = false;
        startPosTimer();
    });

    audioEl.addEventListener("pause", function () {
        btnPlay.hidden = false;
        btnPause.hidden = true;
    });

    audioEl.addEventListener("ended", function () {
        nextTune();
    });

    audioEl.addEventListener("error", function () {
        if (currentTuneId) {
            statusEl.textContent = "⚠ stream error";
        }
    });

    // ── Init ──

    if (tunes.length > 0) buildQueue(false);
    connect();
})();
