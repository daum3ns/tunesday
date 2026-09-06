(function () {
    "use strict";

    var SNIPPET_DURATION = 10;
    var COUNTDOWN_STEPS = 20;
    var FEEDBACK_DELAY = 3500;  // increased from 2000
    var MIN_VIDEO_DURATION = 25;

    var script = document.currentScript;
    var resultPath = script.getAttribute("data-result");
    var dataEl = document.getElementById("quiz-data");
    if (!dataEl) return;
    var DATA = JSON.parse(dataEl.textContent || "{}");
    var ALL_TUNES = (DATA.tunes || []).filter(function (t) { return t.yt && t.yt.length >= 10 && t.provider; });
    var ALL_PROVIDERS = DATA.providers || [];

    var $ = function (id) { return document.getElementById(id); };
    var elLoading = $("quiz-loading");
    var elMenu = $("quiz-menu");
    var elGame = $("quiz-game");
    var elEnd = $("quiz-end");
    var elError = $("quiz-error");
    var elStatus = $("quiz-status");
    var elCBar = $("countdown-bar");
    var elCLabel = $("countdown-label");
    var elAnswers = $("quiz-answers");
    var elFeedback = $("quiz-feedback");
    var elPrompt = $("quiz-prompt");
    var elPlayerW = $("player-wrapper");
    var elAudio = $("quiz-audio");

    var ytPlayer = null;
    var onVideoReady = null;
    var onLoadError = null;
    var loadTimeout = null;
    var roundTimedOut = false;
    var engine = null;
    var currentPlayer = null;
    var selectedMode = null;

    function hideEl(el) { el.hidden = true; }
    function showEl(el) { el.hidden = false; }

    function showMenu() {
        hideEl(elLoading); hideEl(elGame); hideEl(elEnd); hideEl(elError); hideEl(elPlayerW);
        showEl(elMenu);
    }

    function showGame() {
        hideEl(elLoading); hideEl(elMenu); hideEl(elEnd); hideEl(elError);
        showEl(elGame);
        showEl(elPlayerW);
    }

    function showError(msg) {
        hideEl(elLoading); hideEl(elMenu); hideEl(elGame); hideEl(elEnd); hideEl(elPlayerW);
        showEl(elError);
        elError.textContent = msg;
    }

    /* ── Audio Player ── */

    function AudioPlayer(basePath) {
        this.basePath = basePath;
        this.loadTimeout = null;
    }

    AudioPlayer.prototype.setup = function (cb) {
        if (cb) cb();
    };

    AudioPlayer.prototype.loadAndPlay = function (tuneId, cb) {
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

        fetch(self.basePath + "/stream?tune_id=" + tuneId)
            .then(function (res) {
                if (!res.ok) throw new Error("stream " + res.status);
                return res.json();
            })
            .then(function (data) {
                if (settled) return;
                settled = true;
                if (self.loadTimeout) { clearTimeout(self.loadTimeout); self.loadTimeout = null; }
                
                elAudio.src = data.url;
                elAudio.load();
                elAudio.play().catch(function () {
                    // Autoplay blocked by browser, silent fail (user will see it's not playing)
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
        if (elAudio) elAudio.pause();
    };

    /* ── YouTube Player ── */

    function YoutubePlayer() {
        this.loadTimeout = null;
    }

    YoutubePlayer.prototype.setup = function (cb) {
        showEl(elPlayerW);
        ytPlayer = new YT.Player("player", {
            height: "180",
            width: "100%",
            playerVars: { autoplay: 1, controls: 0, enablejsapi: 1, modestbranding: 1, rel: 0 },
            events: {
                onReady: function () { if (cb) cb(); },
                onStateChange: function (ev) {
                    if (ev.data === YT.PlayerState.PLAYING && onVideoReady) {
                        onVideoReady();
                        onVideoReady = null;
                    }
                },
                onError: function () {
                    if (onLoadError) {
                        var fail = onLoadError;
                        onLoadError = null;
                        onVideoReady = null;
                        if (this.loadTimeout) { clearTimeout(this.loadTimeout); this.loadTimeout = null; }
                        fail(new Error("Video unavailable"));
                    }
                }
            }
        });
    };

    YoutubePlayer.prototype.loadAndPlay = function (videoId, cb) {
        var self = this;
        var settled = false;
        onLoadError = cb;
        
        if (self.loadTimeout) clearTimeout(self.loadTimeout);
        self.loadTimeout = setTimeout(function () {
            if (!settled && onLoadError) {
                settled = true;
                onVideoReady = null;
                onLoadError = null;
                self.loadTimeout = null;
                cb(new Error("Video load timed out"));
            }
        }, 15000);

        onVideoReady = function () {
            if (settled) return;
            settled = true;
            if (self.loadTimeout) { clearTimeout(self.loadTimeout); self.loadTimeout = null; }
            onVideoReady = null;
            onLoadError = null;
            var dur = ytPlayer.getDuration();
            if (dur < MIN_VIDEO_DURATION) {
                ytPlayer.stopVideo();
                cb(new Error("Video too short"));
                return;
            }
            var start = Math.floor(dur * (0.10 + Math.random() * 0.70));
            ytPlayer.seekTo(start);
            cb();
        };
        ytPlayer.loadVideoById(videoId);
    };

    YoutubePlayer.prototype.pause = function () {
        if (ytPlayer && typeof ytPlayer.pauseVideo === "function") ytPlayer.pauseVideo();
    };

    /* ── Quiz engine ── */

    function QuizEngine() {
        this.queue = [];
        this.round = 0;
        this.total = 0;
        this.score = 0;
        this.currentTune = null;
        this.countdownTimer = null;
        this.feedbackTimer = null;
        this.guessed = false;
        this.rounds = [];
        this.mode = "quick";
        this.startedAt = null;
        this.playbackMode = "audio";
    }

    QuizEngine.prototype.shuffle = function (arr) {
        var a = arr.slice();
        for (var i = a.length - 1; i > 0; i--) {
            var j = Math.floor(Math.random() * (i + 1));
            var tmp = a[i]; a[i] = a[j]; a[j] = tmp;
        }
        return a;
    };

    QuizEngine.prototype.pickChoices = function (correct, count) {
        var others = this.shuffle(ALL_PROVIDERS.filter(function (p) { return p !== correct; }));
        var picked = [correct].concat(others.slice(0, count - 1));
        return this.shuffle(picked);
    };

    QuizEngine.prototype.renderCountdown = function (remaining) {
        var filled = Math.max(0, Math.round(remaining / SNIPPET_DURATION * COUNTDOWN_STEPS));
        var bar = "";
        for (var i = 0; i < COUNTDOWN_STEPS; i++) bar += i < filled ? "█" : "░";
        elCBar.textContent = bar + " " + remaining + "s";
    };

    QuizEngine.prototype.clearTimers = function () {
        if (this.countdownTimer) { clearInterval(this.countdownTimer); this.countdownTimer = null; }
        if (this.feedbackTimer) { clearTimeout(this.feedbackTimer); this.feedbackTimer = null; }
    };

    QuizEngine.prototype.startGame = function (modeName, rounds, playbackMode) {
        var self = this;
        if (ALL_TUNES.length === 0) { showError("No playable tunes in the team library yet."); return; }
        if (ALL_PROVIDERS.length < 2) { showError("Need at least 2 providers with submitted tunes."); return; }

        var count = Math.min(rounds || ALL_TUNES.length, ALL_TUNES.length);
        var shuffled = self.shuffle(ALL_TUNES);
        self.queue = shuffled.slice(0, count);
        self.round = 0;
        self.total = count;
        self.score = 0;
        self.rounds = [];
        self.mode = modeName;
        self.playbackMode = playbackMode || "audio";
        self.startedAt = new Date().toISOString();

        showGame();
        self.nextRound();
    };

    QuizEngine.prototype.recordRound = function (guess, correct) {
        this.rounds.push({
            tune_id: this.currentTune ? this.currentTune.id : 0,
            guess: guess || "",
            correct: !!correct
        });
        if (correct) this.score++;
    };

    QuizEngine.prototype.nextRound = function () {
        var self = this;
        self.clearTimers();
        if (self.round >= self.total) { self.endGame(); return; }

        self.guessed = false;
        roundTimedOut = false;
        self.currentTune = self.queue[self.round];
        self.round++;

        self.renderCountdown(SNIPPET_DURATION);
        elStatus.textContent = "Round " + self.round + "/" + self.total + "  │  Score: " + self.score;
        elAnswers.innerHTML = "";
        elFeedback.hidden = true;
        elPrompt.hidden = true;
        elCLabel.textContent = "Loading... [";

        var tuneKey = self.playbackMode === "audio" ? self.currentTune.id : self.currentTune.yt;
        currentPlayer.loadAndPlay(tuneKey, function (err) {
            if (err) {
                self.recordRound("", false);
                self.nextRound();
                return;
            }
            elCLabel.textContent = "[";
            elPrompt.hidden = false;
            self.renderChoices();
            self.startCountdown();
        });
    };

    QuizEngine.prototype.renderChoices = function () {
        var self = this;
        var options = self.pickChoices(self.currentTune.provider, Math.min(4, ALL_PROVIDERS.length));
        elAnswers.innerHTML = "";
        options.forEach(function (provider, idx) {
            var btn = document.createElement("button");
            btn.className = "answer-btn";
            btn.textContent = "[" + (idx + 1) + "] " + provider;
            btn.addEventListener("click", function () { self.handleGuess(provider); });
            btn.dataset.num = idx + 1;
            elAnswers.appendChild(btn);
        });
    };

    QuizEngine.prototype.startCountdown = function () {
        var self = this;
        var end = Date.now() + SNIPPET_DURATION * 1000;
        self.countdownTimer = setInterval(function () {
            var remaining = Math.max(0, Math.ceil((end - Date.now()) / 1000));
            self.renderCountdown(remaining);
            if (Date.now() >= end) {
                clearInterval(self.countdownTimer);
                self.countdownTimer = null;
                if (!self.guessed) {
                    roundTimedOut = true;
                    currentPlayer.pause();
                    self.showFeedback(false, null);
                }
            }
        }, 100);
    };

    QuizEngine.prototype.handleGuess = function (provider) {
        if (this.guessed) return;
        this.guessed = true;
        this.clearTimers();
        currentPlayer.pause();
        this.showFeedback(provider === this.currentTune.provider, provider);
    };

    QuizEngine.prototype.showFeedback = function (isCorrect, guessed) {
        var self = this;
        var provider = self.currentTune.provider;
        var tuneName = self.currentTune.name || "Unknown track";

        self.recordRound(guessed, isCorrect);

        if (isCorrect) {
            elFeedback.className = "correct";
            elFeedback.textContent = "✔ Correct! " + provider + " submitted \"" + tuneName + "\"";
        } else if (roundTimedOut) {
            elFeedback.className = "wrong";
            elFeedback.textContent = "⏱ Time's up! " + provider + " submitted \"" + tuneName + "\"";
        } else {
            elFeedback.className = "wrong";
            elFeedback.textContent = "✘ Wrong! " + provider + " submitted \"" + tuneName + "\"";
        }
        elFeedback.hidden = false;
        elStatus.textContent = "Round " + self.round + "/" + self.total + "  │  Score: " + self.score;

        var btns = elAnswers.querySelectorAll(".answer-btn");
        btns.forEach(function (btn) {
            btn.disabled = true;
            var name = btn.textContent.replace(/^\[\d\]\s*/, "");
            if (name === provider) btn.classList.add("correct-highlight");
            else if (guessed && name === guessed) btn.classList.add("wrong-highlight");
        });

        self.feedbackTimer = setTimeout(function () { self.nextRound(); }, FEEDBACK_DELAY);
    };

    QuizEngine.prototype.endGame = function () {
        var self = this;
        self.clearTimers();
        currentPlayer.pause();
        hideEl(elGame);
        hideEl(elPlayerW);
        showEl(elEnd);

        var pct = self.total > 0 ? Math.round(self.score / self.total * 100) : 0;
        var filled = Math.round(pct / 100 * COUNTDOWN_STEPS);
        var bar = "";
        for (var i = 0; i < COUNTDOWN_STEPS; i++) bar += i < filled ? "█" : "░";

        var msg;
        if (pct === 100) msg = "Perfect! You know your team inside out.";
        else if (pct >= 80) msg = "Great ear for your team!";
        else if (pct >= 50) msg = "Not bad! Keep listening.";
        else msg = "Time to attend more tunesday sessions...";

        elEnd.innerHTML =
            "<pre>GAME OVER</pre>" +
            "<pre>" + self.score + " / " + self.total + " correct</pre>" +
            "<pre>" + bar + " " + pct + "%</pre>" +
            "<pre>" + msg + "</pre>" +
            "<pre id='quiz-save-state'>transmitting score to the server…</pre>" +
            "<button class='end-btn'>[ Play again ]</button>";
        elEnd.querySelector(".end-btn").addEventListener("click", function () { self.reset(); });

        fetch(resultPath, {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({
                mode: self.mode,
                started_at: self.startedAt,
                score: self.score,
                total: self.total,
                rounds: self.rounds.map(function (r) {
                    return { tune_id: r.tune_id, guess: r.guess, correct: r.correct };
                })
            })
        }).then(function (res) {
            return res.ok ? res.json() : Promise.reject(new Error("HTTP " + res.status));
        }).then(function (out) {
            var note = $("quiz-save-state");
            if (note) {
                note.textContent = "✔ saved to the leaderboard: " + out.score + "/" + out.total +
                    (out.score === self.score ? " — the judges agree" : " (your count differed, ours rules)");
            }
        }).catch(function (err) {
            var note = $("quiz-save-state");
            if (note) note.textContent = "✘ score not saved (" + err.message + ")";
        });
    };

    QuizEngine.prototype.reset = function () {
        this.clearTimers();
        currentPlayer.pause();
        buildMenu();
        showMenu();
    };

    function buildMenu() {
        var total = ALL_TUNES.length;
        elMenu.innerHTML =
            '<span class="title mono">GUESS THE PROVIDER</span>' +
            "<span class='mono'>" + total + " playable tunes from " + ALL_PROVIDERS.length + " providers</span>" +
            "<span class='mono' id='mode-selector'><strong>Playback:</strong> " +
            "<button class='menu-btn mode-btn' data-playback='audio'>🔊 audio</button> " +
            "<button class='menu-btn mode-btn' data-playback='video'>🎬 video</button></span>" +
            "<button class='menu-btn' data-mode='quick' data-rounds='5'>[ Quick Game (5) ]</button>" +
            "<button class='menu-btn' data-mode='universe' data-rounds='42'>[ Life, Universe &amp; Everything (42) ]</button>" +
            "<button class='menu-btn' data-mode='all'>[ All (" + total + " tunes) ]</button>" +
            "<p id='menu-status' class='muted' hidden></p>";
        
        var elMenuStatus = elMenu.querySelector("#menu-status");
        var buttons = elMenu.querySelectorAll(".menu-btn:not(.mode-btn)");
        var modeButtons = elMenu.querySelectorAll(".mode-btn");

        // Set initial selected mode
        selectedMode = localStorage.getItem("quiz-playback-mode") || "audio";
        modeButtons.forEach(function (btn) {
            if (btn.dataset.playback === selectedMode) {
                btn.classList.add("active");
            }
        });

        // Mode selection
        modeButtons.forEach(function (btn) {
            btn.addEventListener("click", function () {
                modeButtons.forEach(function (b) { b.classList.remove("active"); });
                btn.classList.add("active");
                selectedMode = btn.dataset.playback;
                localStorage.setItem("quiz-playback-mode", selectedMode);
            });
        });

        function setBusy(busy) {
            Array.prototype.forEach.call(buttons, function (b) { b.disabled = busy; });
            Array.prototype.forEach.call(modeButtons, function (b) { b.disabled = busy; });
            elMenuStatus.hidden = false;
            if (selectedMode === "video") {
                elMenuStatus.textContent = busy
                    ? "arming the player…"
                    : "the YouTube player did not load. an ad/tracker blocker, hardened browser " +
                      "(Brave Shields, strict tracking protection) or a company proxy/CA setup is " +
                      "blocking youtube.com. allow it for this site and click again.";
            } else {
                elMenuStatus.textContent = busy ? "starting audio quiz…" : "";
                elMenuStatus.hidden = !busy;
            }
        }

        Array.prototype.forEach.call(buttons, function (btn) {
            btn.addEventListener("click", function () {
                if (selectedMode === "video") {
                    setBusy(true);
                    ensureVideoPlayer(function () {
                        setBusy(false);
                        elMenuStatus.hidden = true;
                        currentPlayer = new YoutubePlayer();
                        engine.startGame(btn.dataset.mode, btn.dataset.rounds ? parseInt(btn.dataset.rounds, 10) : 0, "video");
                    }, function () { setBusy(false); });
                } else {
                    currentPlayer = new AudioPlayer("/teams/" + (new URL(window.location).pathname.split("/")[2]) + "/radio");
                    currentPlayer.setup();
                    engine.startGame(btn.dataset.mode, btn.dataset.rounds ? parseInt(btn.dataset.rounds, 10) : 0, "audio");
                }
            });
        });
    }

    /* ── YouTube API loading (only for video mode) ── */

    var playerState = "idle"; // idle | loading | ready
    var playerQueue = [];
    var playerWatchdog = null;

    function onApiReady(fn) {
        if (window.YT && YT.Player) { fn(); return; }
        var prev = window.onYouTubeIframeAPIReady;
        window.onYouTubeIframeAPIReady = function () {
            if (typeof prev === "function") { try { prev(); } catch (e) {} }
            fn();
        };
    }

    function ensureVideoPlayer(cb, onFail) {
        if (playerState === "ready") return cb();
        playerQueue.push({ cb: cb, onFail: onFail });
        if (playerState === "loading") return;

        playerState = "loading";
        if (playerWatchdog) clearTimeout(playerWatchdog);
        playerWatchdog = setTimeout(function () {
            if (playerState !== "loading") return;
            playerState = "idle"; // next click retries from scratch
            var q = playerQueue; playerQueue = [];
            q.forEach(function (item) { if (item.onFail) item.onFail(); });
        }, 8000);

        var go = function () {
            var player = new YoutubePlayer();
            player.setup(function () {
                playerState = "ready";
                if (playerWatchdog) { clearTimeout(playerWatchdog); playerWatchdog = null; }
                var q = playerQueue.slice();
                playerQueue = [];
                q.forEach(function (item) { item.cb(); });
            });
        };

        if (window.YT && YT.Player) { go(); return; }
        onApiReady(go);
        var s = document.createElement("script");
        s.src = "https://www.youtube.com/iframe_api";
        document.head.appendChild(s);
    }

    document.addEventListener("keydown", function (e) {
        if (!engine || !engine.currentTune || engine.guessed || elGame.hidden) return;
        var num = parseInt(e.key, 10);
        if (num >= 1 && num <= 4) {
            var target = null;
            elAnswers.querySelectorAll(".answer-btn").forEach(function (b) {
                if (parseInt(b.dataset.num, 10) === num) target = b;
            });
            if (target && !target.disabled) target.click();
        }
    });

    if (ALL_TUNES.length === 0 || ALL_PROVIDERS.length < 2) {
        hideEl(elLoading);
        showError("Not enough material yet: the quiz needs playable tunes from at least 2 providers.");
    } else {
        hideEl(elLoading);
        engine = window.quizEngine = new QuizEngine();
        buildMenu();
        showMenu();
    }
})();
