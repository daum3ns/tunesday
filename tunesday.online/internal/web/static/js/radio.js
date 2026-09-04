(function () {
    "use strict";

    var script = document.currentScript;
    var wsPath = script.getAttribute("data-ws");
    var basePath = script.getAttribute("data-base");

    var statusEl  = document.getElementById("radio-status");
    var trackEl   = document.getElementById("radio-track");
    var metaEl    = document.getElementById("radio-meta");
    var posEl     = document.getElementById("radio-pos");
    var nowPlayingBox = document.getElementById("radio-now-playing");
    var progressFill = document.getElementById("radio-progress-fill");
    var audioEl   = document.getElementById("radio-audio");
    var listenersEl = document.getElementById("radio-listeners");
    var btnPlay   = document.getElementById("btn-play");
    var btnPause  = document.getElementById("btn-pause");
    var btnNext   = document.getElementById("btn-next");
    var btnPrev   = document.getElementById("btn-prev");
    var btnShuffle = document.getElementById("btn-shuffle");
    var volSlider = document.getElementById("radio-volume");
    var volLabel  = document.getElementById("radio-vol-label");
    var volIcon   = document.getElementById("radio-vol-icon");

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
    var loadedTuneId = 0;
    var isLoading = false;
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

    function setVolume(pct) {
        if (pct < 0) pct = 0;
        if (pct > 100) pct = 100;
        audioEl.volume = pct / 100;
        if (volLabel) volLabel.textContent = pct + "%";
        if (volSlider && volSlider.value !== String(pct)) volSlider.value = pct;
        if (volIcon) {
            if (pct === 0) volIcon.textContent = "🔇";
            else if (pct < 50) volIcon.textContent = "🔉";
            else volIcon.textContent = "🔊";
        }
    }

    function initVolume() {
        var saved = null;
        try { saved = parseInt(localStorage.getItem("radioVolume"), 10); } catch (e) {}
        if (volSlider && saved !== null && !isNaN(saved)) {
            setVolume(saved);
        } else if (volSlider) {
            setVolume(parseInt(volSlider.value, 10));
        }
    }

    function loadAndPlay(tuneId) {
        if (!tuneId) return;
        currentTuneId = tuneId;
        loadedTuneId = 0;
        isLoading = true;
        resetProgress();
        statusEl.textContent = "loading…";
        var meta = tuneMeta(tuneId);
        trackEl.textContent = "♪ " + meta.title;
        setMeta(meta);
        startPosTimer();

        fetch(basePath + "/stream?tune_id=" + tuneId)
            .then(function (res) {
                if (!res.ok) throw new Error("stream " + res.status);
                return res.json();
            })
            .then(function (data) {
                if (currentTuneId !== tuneId) { isLoading = false; return; }
                audioEl.src = data.url;
                loadedTuneId = tuneId;
                isLoading = false;
                audioEl.load();
                audioEl.play().then(function () {
                    statusEl.textContent = "▶ live";
                    trackEl.textContent = "♪ " + tuneMeta(tuneId).title;
                    reportNowPlaying(tuneId);
                }).catch(function () {
                    statusEl.textContent = "▶ live (click play to unmute)";
                });
            })
            .catch(function () {
                isLoading = false;
                statusEl.textContent = "⚠ stream unavailable";
            });
    }

    function tuneMeta(id) {
        // Fallback: scan the table rows for title / provider / date.
        var rows = document.querySelectorAll("tr[data-tune]");
        for (var i = 0; i < rows.length; i++) {
            if (parseInt(rows[i].getAttribute("data-tune"), 10) === id) {
                var cells = rows[i].querySelectorAll("td");
                var title = cells.length > 1 ? cells[1].textContent : "unknown";
                var provider = rows[i].getAttribute("data-provider") || "";
                var added = rows[i].getAttribute("data-added") || "";
                return { title: title, provider: provider, added: added };
            }
        }
        return { title: "unknown", provider: "", added: "" };
    }

    function setMeta(meta) {
        if (!metaEl) return;
        if (meta.provider || meta.added) {
            var parts = [];
            if (meta.provider) parts.push("provided by " + meta.provider);
            if (meta.added) parts.push(meta.added);
            metaEl.textContent = parts.join(" · ");
        } else {
            metaEl.textContent = "";
        }
    }

    function startPosTimer() {
        if (posTimer) clearInterval(posTimer);
        posTimer = setInterval(function () {
            if (!audioEl.duration) {
                if (audioEl.paused) { posEl.textContent = ""; progressFill.style.width = "0%"; }
                return;
            }
            var pos = audioEl.currentTime;
            if (audioEl.paused) return; // keep frozen; blink is handled via class
            posEl.textContent = fmt(pos) + " / " + fmt(audioEl.duration);
            progressFill.style.width = (pos / audioEl.duration * 100) + "%";
        }, 250);
    }

    function resetProgress() {
        posEl.textContent = "";
        progressFill.style.width = "0%";
        if (nowPlayingBox) nowPlayingBox.classList.remove("blinking");
    }

    function blinkPausedOn() {
        if (!nowPlayingBox) return;
        // Keep the current frozen position text.
        if (!posEl.textContent && audioEl.duration) {
            posEl.textContent = fmt(audioEl.currentTime) + " / " + fmt(audioEl.duration);
            progressFill.style.width = (audioEl.currentTime / audioEl.duration * 100) + "%";
        }
        nowPlayingBox.classList.add("blinking");
    }

    function blinkPausedOff() {
        if (nowPlayingBox) nowPlayingBox.classList.remove("blinking");
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
        if (isLoading) return;
        if (currentTuneId && loadedTuneId === currentTuneId && audioEl.src) {
            audioEl.play().then(function () {
                statusEl.textContent = "▶ live";
                blinkPausedOff();
            }).catch(function () {});
            return;
        }
        if (queue.length === 0) buildQueue(false);
        loadAndPlay(queue[0]);
    });

    btnPause.addEventListener("click", function () {
        audioEl.pause();
        statusEl.textContent = "❚❚ paused";
        blinkPausedOn();
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
        blinkPausedOff();
        startPosTimer();
    });

    audioEl.addEventListener("pause", function () {
        btnPlay.hidden = false;
        btnPause.hidden = true;
        blinkPausedOn();
    });

    audioEl.addEventListener("ended", function () {
        nextTune();
    });

    audioEl.addEventListener("error", function () {
        if (currentTuneId) {
            statusEl.textContent = "⚠ stream error";
        }
    });

    // ── Volume ──

    if (volSlider) {
        volSlider.addEventListener("input", function () {
            setVolume(parseInt(this.value, 10));
            try { localStorage.setItem("radioVolume", String(this.value)); } catch (e) {}
        });
    }

    // ── Init ──

    if (tunes.length > 0) buildQueue(false);
    initVolume();
    connect();
})();
