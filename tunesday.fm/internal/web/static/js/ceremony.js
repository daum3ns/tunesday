(function () {
    "use strict";

    var script = document.currentScript;
    var wsPath = script.getAttribute("data-ws");
    var revealPath = script.getAttribute("data-reveal");

    var statusEl = document.getElementById("drum-status");
    var turntable = document.getElementById("turntable");
    var attendeesEl = document.getElementById("attendees");
    var revealBtn = document.getElementById("reveal-btn");
    var hostHint = document.getElementById("host-hint");
    var winnerZone = document.getElementById("winner-zone");
    var bigWinner = document.getElementById("big-winner");
    var tuneZone = document.getElementById("tune-zone");
    var completeInfo = document.getElementById("complete-info");
    var sessionUrl = document.getElementById("session-url");
    var copyLink = document.getElementById("copy-link");
    var countdownEl = document.getElementById("countdown");
    var muteBtn = document.getElementById("mute-btn");

    var myProvider = null;
    var revealed = false;
    var ws = null;
    var currentPool = [];
    var muted = (function () {
        try { return localStorage.getItem("tunesday-muted") === "1"; } catch (e) { return false; }
    })();

    if (sessionUrl) sessionUrl.textContent = location.href.replace(/\/host\/?$/, "");
    if (copyLink) copyLink.addEventListener("click", function () {
        navigator.clipboard.writeText(sessionUrl.textContent).then(function () {
            copyLink.textContent = "copied!";
            setTimeout(function () { copyLink.textContent = "copy"; }, 1500);
        });
    });

    // ---- sound: terminal-grade percussion, all Web Audio, no assets ----

    function audioCtx() {
        var AC = window.AudioContext || window.webkitAudioContext;
        if (!AC) return null;
        if (!audioCtx.ctx) audioCtx.ctx = new AC();
        return audioCtx.ctx;
    }

    function tick(ctx, when) {
        var osc = ctx.createOscillator();
        var gain = ctx.createGain();
        osc.type = "square";
        osc.frequency.value = 90 + Math.random() * 60;
        gain.gain.setValueAtTime(0.08, when);
        gain.gain.exponentialRampToValueAtTime(0.001, when + 0.04);
        osc.connect(gain).connect(ctx.destination);
        osc.start(when);
        osc.stop(when + 0.05);
    }

    function drumroll(duration) {
        if (muted) return;
        var ctx = audioCtx();
        if (!ctx) return;
        if (ctx.state === "suspended") ctx.resume();
        var t = ctx.currentTime;
        var end = t + duration / 1000 * 0.9;
        var step = 0.035;
        while (t < end) {
            tick(ctx, t);
            t += step;
            step *= 0.996; // accelerate
        }
    }

    function tock() {
        if (muted) return;
        var ctx = audioCtx();
        if (!ctx) return;
        if (ctx.state === "suspended") ctx.resume();
        var osc = ctx.createOscillator();
        var gain = ctx.createGain();
        osc.type = "square";
        osc.frequency.value = 440;
        var now = ctx.currentTime;
        gain.gain.setValueAtTime(0.12, now);
        gain.gain.exponentialRampToValueAtTime(0.001, now + 0.12);
        osc.connect(gain).connect(ctx.destination);
        osc.start(now);
        osc.stop(now + 0.13);
    }

    function fanfare() {
        if (muted) return;
        var ctx = audioCtx();
        if (!ctx) return;
        if (ctx.state === "suspended") ctx.resume();
        [523.25, 659.25, 783.99, 1046.5].forEach(function (f, i) {
            var osc = ctx.createOscillator();
            var gain = ctx.createGain();
            osc.type = "triangle";
            osc.frequency.value = f;
            var when = ctx.currentTime + i * 0.16;
            gain.gain.setValueAtTime(0.15, when);
            gain.gain.exponentialRampToValueAtTime(0.001, when + 0.5);
            osc.connect(gain).connect(ctx.destination);
            osc.start(when);
            osc.stop(when + 0.55);
        });
    }

    function speak(text) {
        if (muted || !("speechSynthesis" in window)) return;
        try {
            var u = new SpeechSynthesisUtterance(text);
            u.rate = 0.85;
            u.pitch = 0.4;
            speechSynthesis.speak(u);
        } catch (e) { /* the robot stays silent */ }
    }

    function updateMuteBtn() {
        if (!muteBtn) return;
        muteBtn.textContent = muted ? "♪ muted" : "♫ sound";
        muteBtn.classList.toggle("active", !muted);
    }
    if (muteBtn) {
        muteBtn.addEventListener("click", function () {
            muted = !muted;
            try { localStorage.setItem("tunesday-muted", muted ? "1" : "0"); } catch (e) {}
            updateMuteBtn();
            if (!muted) tock(); // audible confirmation that sound is back
        });
    }
    updateMuteBtn();

    function confetti() {
        var chars = ["█", "▓", "▒", "░", "♪", "♫"];
        for (var i = 0; i < 80; i++) {
            var s = document.createElement("span");
            s.className = "confetti";
            s.textContent = chars[Math.floor(Math.random() * chars.length)];
            s.style.left = Math.random() * 100 + "vw";
            s.style.animationDuration = (2 + Math.random() * 2.5) + "s";
            s.style.animationDelay = (Math.random() * 0.7) + "s";
            s.style.fontSize = (10 + Math.random() * 18) + "px";
            document.body.appendChild(s);
            setTimeout(function (el) { return function () { el.remove(); }; }(s), 5000);
        }
    }

    // ---- transport ----

    function connect() {
        var proto = location.protocol === "https:" ? "wss://" : "ws://";
        ws = new WebSocket(proto + location.host + wsPath);
        ws.onmessage = function (ev) {
            handleMessage(JSON.parse(ev.data));
        };
        ws.onclose = function () {
            statusEl.textContent = "broadcast lost… reconnecting";
            setTimeout(connect, 2000);
        };
    }

    function handleMessage(msg) {
        if (msg.type === "state") applyState(msg.payload);
        else if (msg.type === "attendees") onAttendees(msg.payload);
        else if (msg.type === "reveal") runRoulette(msg.payload);
        else if (msg.type === "complete") showCompleted(msg.payload);
    }

    function applyState(st) {
        if (st.status === "open") currentPool = st.poolPreview || [];
        renderAttendees(st.attendees || []);
        for (var i = 0; i < (st.attendees || []).length; i++) {
            if (st.attendees[i].isYou) myProvider = st.attendees[i].provider;
        }
        if (st.status === "open") {
            if (revealBtn) setRevealEnabled(!!st.canReveal);
            statusEl.textContent = roomMessage(st.inRoom || 0, st.poolPreview);
        } else {
            revealed = true;
            winnerZone.hidden = false;
            bigWinner.textContent = winnerBanner(st.winner || "?");
            statusEl.textContent = "the verdict is in.";
            if (st.youWin || st.canAddTune) tuneZone.hidden = !st.canAddTune;
            if (st.status === "completed") showCompletedStatic(st);
            hideReveal();
        }
    }

    function roomMessage(inRoom, pool) {
        var base = inRoom + " in the room";
        if (pool && pool.length) base += " · candidates: " + pool.join(", ");
        return base;
    }

    function onAttendees(p) {
        if (!revealed) currentPool = p.poolPreview || [];
        renderAttendees(p.attendees || []);
        if (revealBtn && !revealed) setRevealEnabled(!!p.revealReady);
        if (!revealed) statusEl.textContent = roomMessage(p.inRoom || 0, p.poolPreview);
    }

    function setRevealEnabled(on) {
        revealBtn.disabled = !on;
        hostHint.textContent = on
            ? "the room is charged. drop it."
            : "need at least 2 eligible providers connected";
    }

    function renderAttendees(list) {
        attendeesEl.innerHTML = "";
        turntable.innerHTML = "";
        if (!list.length) {
            var empty = document.createElement("li");
            empty.className = "muted";
            empty.textContent = "nobody connected yet — be the first record!";
            attendeesEl.appendChild(empty);
            return;
        }
        list.forEach(function (a) {
            var li = document.createElement("li");
            li.className = a.live ? "" : "muted";
            var mark = a.live ? "⏻" : "○";
            li.textContent = mark + " " + a.alias + (a.provider ? "  [ " + a.provider + " ]" : "")
                + (a.isYou ? "  ← you" : "") + (a.live ? "" : "  (left)");
            attendeesEl.appendChild(li);

            var disc = document.createElement("div");
            var provider = a.provider || a.alias;
            disc.className = "vinyl" + (a.live ? "" : " vinyl-offline")
                + (a.live && currentPool.indexOf(provider) !== -1 ? " candidate" : "");
            disc.dataset.provider = provider;
            disc.dataset.live = a.live ? "1" : "0";
            var label = document.createElement("span");
            label.className = "vinyl-label";
            label.textContent = provider;
            disc.appendChild(label);
            turntable.appendChild(disc);
        });
    }

    if (revealBtn) revealBtn.addEventListener("click", function () {
        revealBtn.disabled = true;
        fetch(revealPath, { method: "POST" }).then(function (res) {
            if (!res.ok) {
                return res.text().then(function (t) {
                    statusEl.textContent = (t || "the needle jammed").trim();
                    setRevealEnabled(false);
                    setTimeout(function () { setRevealEnabled(true); }, 1500);
                });
            }
            // the broadcast will carry the reveal to every screen
        });
    });

    // The synchronized ridiculous part: winner is already locked server-side;
    // we only stage the theatre — a 3-2-1 countdown, then the roulette.
    function runRoulette(payload) {
        revealed = true;
        hideReveal();
        currentPool = [];
        Array.prototype.forEach.call(turntable.querySelectorAll(".vinyl"), function (d) {
            d.classList.remove("candidate");
        });
        var pool = payload.pool || [];
        var winner = payload.winner;
        var duration = payload.duration_ms || 2500;
        var countdown = typeof payload.countdown_ms === "number" ? payload.countdown_ms : 0;

        var discs = Array.prototype.slice.call(turntable.querySelectorAll(".vinyl"));
        var candidates = discs.filter(function (d) {
            return pool.indexOf(d.dataset.provider) !== -1;
        });
        if (!candidates.length) candidates = discs;

        if (countdown > 0) {
            startCountdown(Math.ceil(countdown / 1000), function () {
                shuffle(candidates, winner, pool, duration);
            });
        } else {
            shuffle(candidates, winner, pool, duration);
        }
    }

    function startCountdown(n, done) {
        if (!countdownEl) countdownEl.hidden = false;
        function step(value) {
            if (value <= 0) {
                if (countdownEl) countdownEl.hidden = true;
                done();
                return;
            }
            if (countdownEl) countdownEl.textContent = value;
            tock();
            statusEl.textContent = value + "… the needle hangs above the wax…";
            setTimeout(function () { step(value - 1); }, 1000);
        }
        step(n);
    }

    function shuffle(candidates, winner, pool, duration) {
        drumroll(duration);
        statusEl.textContent = "the needle descends… the records tremble…";

        var start = performance.now();
        var idx = Math.floor(Math.random() * candidates.length);
        function shuffleStep() {
            var elapsed = performance.now() - start;
            if (elapsed >= duration) {
                landOn(candidates, winner, pool);
                return;
            }
            candidates.forEach(function (d) { d.classList.remove("spinning-fast"); });
            idx = (idx + 1 + Math.floor(Math.random() * (candidates.length - 1))) % candidates.length;
            candidates[idx].classList.add("spinning-fast");
            // ease-out: the shuffle slows down as the needle approaches
            var progress = elapsed / duration;
            var delay = 40 + progress * progress * 600;
            setTimeout(shuffleStep, delay);
        }
        shuffleStep();
    }

    function landOn(candidates, winner, pool) {
        candidates.forEach(function (d) {
            d.classList.remove("spinning-fast");
            if (d.dataset.provider === winner) {
                d.classList.add("winner-record");
            } else if (pool.indexOf(d.dataset.provider) !== -1) {
                d.classList.add("loser-record");
            }
        });
        winnerZone.hidden = false;
        bigWinner.textContent = winnerBanner(winner);
        statusEl.textContent = "the algorithm has spoken.";
        confetti();
        fanfare();
        speak("The Algorithm has spoken. Today, " + winner + ", shall provide the tunes.");
        if (myProvider && myProvider === winner) {
            tuneZone.hidden = false;
        }
    }

    function showCompleted(payload) {
        tuneZone.hidden = true;
        completeInfo.hidden = false;
        completeInfo.textContent = "♪ registered: " + payload.title + " provided by " + payload.provider + ". Happy Tunesday!";
    }

    function showCompletedStatic(st) {
        if (st.tuneTitle) {
            completeInfo.hidden = false;
            completeInfo.textContent = "♪ registered: " + st.tuneTitle;
        }
    }

    function hideReveal() {
        if (revealBtn) {
            revealBtn.disabled = true;
            revealBtn.style.display = "none";
            if (hostHint) hostHint.style.display = "none";
        }
    }

    function winnerBanner(name) {
        var inner = "*** THE ALGORITHM HAS SPOKEN ***";
        var line = "═".repeat(Math.max(inner.length, name.length) + 8);
        return line + "\n" +
            "   >>  " + inner + "  <<\n" +
            "        ▼ ▼ ▼\n" +
            "    ★ " + name.toUpperCase() + " ★\n" +
            "   provides today's soundtrack\n" +
            line;
    }

    connect();
})();
