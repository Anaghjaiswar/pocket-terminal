(() => {
  "use strict";

  const connBadge = document.getElementById("connBadge");
  const connText = document.getElementById("connText");
  const voiceBtn = document.getElementById("voiceBtn");
  const voiceBadge = document.getElementById("voiceBadge");
  const promptModalBtn = document.getElementById("promptModalBtn");
  const promptDrawer = document.getElementById("promptDrawer");
  const drawerCloseBtn = document.getElementById("drawerCloseBtn");
  const drawerMicBtn = document.getElementById("drawerMicBtn");
  const drawerSendBtn = document.getElementById("drawerSendBtn");
  const promptInput = document.getElementById("promptInput");
  const autoEnterCheck = document.getElementById("autoEnterCheck");
  const ctrlToggle = document.getElementById("ctrlToggle");
  const altToggle = document.getElementById("altToggle");

  const shellPickerBtn = document.getElementById("shellPickerBtn");
  const shellModal = document.getElementById("shellModal");
  const shellModalClose = document.getElementById("shellModalClose");
  const activeShellIcon = document.getElementById("activeShellIcon");
  const activeShellName = document.getElementById("activeShellName");

  const SHELL_META = {
    powershell: { name: "PowerShell", icon: "⚡" },
    gitbash: { name: "Git Bash", icon: "🐙" },
    cmd: { name: "CMD", icon: "💻" },
    wsl: { name: "WSL", icon: "🐧" },
  };

  const urlParams = new URLSearchParams(window.location.search);
  let currentShell = urlParams.get("shell") || localStorage.getItem("pocket_shell") || "gitbash";
  const projectPath = document.getElementById("projectPath");
  const sessionTabs = document.getElementById("sessionTabs");
  const sessionError = document.getElementById("sessionError");
  let project = urlParams.get("project") || localStorage.getItem("pocket_project") || "";
  let sessions = [];
  let activeSession = null;
  let generation = 0;
  let reconnectAttempts = 0;
  let busy = false;

  let isCtrlActive = false;
  let isAltActive = false;

  // 1. Initialize xterm.js with Catppuccin Mocha developer theme
  const term = new window.Terminal({
    cursorBlink: true,
    cursorStyle: "block",
    convertEol: true,
    scrollback: 5000,
    fontFamily: '"JetBrains Mono", "Cascadia Code", "Fira Code", Consolas, monospace',
    fontSize: window.innerWidth < 600 ? 13 : 14,
    lineHeight: 1.2,
    theme: {
      background: "#1e1e2e",
      foreground: "#cdd6f4",
      cursor: "#f5e0dc",
      cursorAccent: "#1e1e2e",
      selectionBackground: "rgba(203, 166, 247, 0.28)",
      black: "#45475a",
      red: "#f38ba8",
      green: "#a6e3a1",
      yellow: "#f9e2af",
      blue: "#89b4fa",
      magenta: "#f5c2e7",
      cyan: "#94e2d5",
      white: "#bac2de",
      brightBlack: "#585b70",
      brightRed: "#f38ba8",
      brightGreen: "#a6e3a1",
      brightYellow: "#f9e2af",
      brightBlue: "#89b4fa",
      brightMagenta: "#f5c2e7",
      brightCyan: "#94e2d5",
      brightWhite: "#a6adc8",
    },
  });

  const fitAddon = new window.FitAddon.FitAddon();
  term.loadAddon(fitAddon);
  term.open(document.getElementById("terminal"));

  let socket = null;
  let reconnectTimer = null;

  function setStatus(state, label) {
    connBadge.className = "conn-badge " + state;
    connText.textContent = label;
  }

  function updateShellUI(shellKey) {
    currentShell = shellKey;
    localStorage.setItem("pocket_shell", shellKey);
    const meta = SHELL_META[shellKey] || { name: shellKey, icon: "🐚" };
    activeShellIcon.textContent = meta.icon;
    activeShellName.textContent = meta.name;

    document.querySelectorAll(".shell-card").forEach((card) => {
      if (card.getAttribute("data-shell") === shellKey) {
        card.classList.add("active");
      } else {
        card.classList.remove("active");
      }
    });
  }

  function buildWsUrl() {
    const proto = window.location.protocol === "https:" ? "wss:" : "ws:";
    const params = new URLSearchParams(window.location.search);
    const token = params.get("token") || "";
    return `${proto}//${window.location.host}/ws?token=${encodeURIComponent(token)}&session=${encodeURIComponent(activeSession.id)}`;
  }

  async function api(method, params = {}, body, resource = "sessions") {
    const query = new URLSearchParams(params);
    if (urlParams.get("token")) query.set("token", urlParams.get("token"));
    const response = await fetch(`/api/${resource}?${query}`, {
      method,
      headers: { "Content-Type": "application/json" },
      body: body === undefined ? undefined : JSON.stringify(body),
    });
    if (!response.ok) throw new Error(await response.text());
    return response.status === 204 ? null : response.json();
  }

  function showError(error) {
    sessionError.textContent = error ? error.message : "";
    sessionError.classList.toggle("hidden", !error);
  }

  async function runAction(action) {
    if (busy) return;
    busy = true;
    showError(null);
    try { await action(); } catch (error) { showError(error); }
    finally { busy = false; }
  }

  function detachSocket() {
    generation++;
    clearTimeout(reconnectTimer);
    reconnectTimer = null;
    if (socket) {
      const old = socket;
      socket = null;
      old.close(1000, "Switching terminal");
    }
  }

  function renderSessions() {
    sessionTabs.replaceChildren();
    sessions.forEach((session, index) => {
      const tab = document.createElement("div");
      tab.className = "session-tab" + (activeSession?.id === session.id ? " active" : "");
      const select = document.createElement("button");
      select.type = "button";
      select.textContent = `${SHELL_META[session.shell]?.name || session.shell} ${index + 1}${session.exited ? " (exited)" : ""}`;
      select.setAttribute("aria-pressed", String(activeSession?.id === session.id));
      select.addEventListener("click", () => { if (!busy) activateSession(session); });
      const close = document.createElement("button");
      close.type = "button";
      close.textContent = "×";
      close.title = "Terminate this terminal and its running shell";
      close.setAttribute("aria-label", `Terminate terminal ${index + 1}`);
      close.addEventListener("click", () => runAction(async () => {
        await api("DELETE", { id: session.id });
        sessions = sessions.filter((item) => item.id !== session.id);
        if (activeSession?.id === session.id) activateSession(sessions[0] || null);
        else renderSessions();
      }));
      tab.append(select, close);
      sessionTabs.append(tab);
    });
  }

  function activateSession(session) {
    detachSocket();
    activeSession = session;
    reconnectAttempts = 0;
    term.reset();
    renderSessions();
    if (!session) { setStatus("disconnected", "No terminal"); return; }
    localStorage.setItem(`pocket_active:${project}`, session.id);
    updateShellUI(session.shell);
    connectWebSocket();
  }

  async function loadProject(path) {
    const result = await api("GET", { project: path });
    project = result.project;
    projectPath.value = project;
    localStorage.setItem("pocket_project", project);
    sessions = result.sessions;
    const saved = localStorage.getItem(`pocket_active:${project}`);
    activateSession(sessions.find((item) => item.id === saved) || sessions[0] || null);
  }

  document.getElementById("projectForm").addEventListener("submit", (event) => {
    event.preventDefault();
    runAction(() => loadProject(projectPath.value.trim()));
  });
  document.getElementById("newSessionBtn").addEventListener("click", openShellModal);

  const folderPicker = document.getElementById("folderPicker");
  const folderDrive = document.getElementById("folderDrive");
  const folderPath = document.getElementById("folderPath");
  const folderList = document.getElementById("folderList");
  const folderMessage = document.getElementById("folderMessage");
  const folderUp = document.getElementById("folderUp");
  const selectFolderBtn = document.getElementById("selectFolderBtn");
  let browseDirectory = "";
  let browseParent = "";
  let browseVersion = 0;

  function closeFolderPicker() {
    browseVersion++;
    folderPicker.classList.add("hidden");
    document.getElementById("browseProjectBtn").focus();
  }

  async function browseFolder(path) {
    const version = ++browseVersion;
    selectFolderBtn.disabled = true;
    folderUp.disabled = true;
    folderList.replaceChildren();
    folderMessage.textContent = "Loading folders…";
    try {
      const listing = await api("GET", { path }, undefined, "directories");
      if (version !== browseVersion) return;
      browseDirectory = listing.path;
      browseParent = listing.parent;
      folderPath.value = listing.path;
      folderDrive.replaceChildren();
      const currentRoot = listing.roots.find((root) =>
        listing.path.toLowerCase().startsWith(root.path.toLowerCase()));
      if (!currentRoot) {
        const option = document.createElement("option");
        option.value = "";
        option.textContent = "Other location";
        folderDrive.append(option);
      }
      listing.roots.forEach((root) => {
        const option = document.createElement("option");
        option.value = root.path;
        option.textContent = root.name;
        folderDrive.append(option);
      });
      folderDrive.value = currentRoot?.path || "";
      listing.directories.forEach((directory) => {
        const button = document.createElement("button");
        button.type = "button";
        button.textContent = `📁 ${directory.name}`;
        button.addEventListener("click", () => browseFolder(directory.path));
        folderList.append(button);
      });
      folderMessage.textContent = listing.directories.length ? "Tap a folder to browse inside it." : "No subfolders. You can select this folder.";
      folderUp.disabled = !browseParent;
      selectFolderBtn.disabled = false;
    } catch (error) {
      if (version !== browseVersion) return;
      folderMessage.textContent = error.message;
      folderUp.disabled = !browseParent;
    }
  }

  document.getElementById("browseProjectBtn").addEventListener("click", () => {
    if (busy) return;
    folderPicker.classList.remove("hidden");
    folderDrive.focus();
    browseFolder(project || "");
  });
  document.getElementById("folderPickerClose").addEventListener("click", closeFolderPicker);
  folderPicker.addEventListener("click", (event) => {
    if (event.target === folderPicker) closeFolderPicker();
  });
  window.addEventListener("keydown", (event) => {
    if (event.key === "Escape" && !folderPicker.classList.contains("hidden")) closeFolderPicker();
    if (event.key === "Tab" && !folderPicker.classList.contains("hidden")) {
      const controls = [...folderPicker.querySelectorAll("button:not(:disabled), input, select")];
      const first = controls[0];
      const last = controls[controls.length - 1];
      if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last.focus(); }
      else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first.focus(); }
    }
  });
  folderDrive.addEventListener("change", () => { if (folderDrive.value) browseFolder(folderDrive.value); });
  folderUp.addEventListener("click", () => { if (browseParent) browseFolder(browseParent); });
  document.getElementById("folderPathForm").addEventListener("submit", (event) => {
    event.preventDefault();
    browseFolder(folderPath.value.trim());
  });
  selectFolderBtn.addEventListener("click", () => runAction(async () => {
    if (!browseDirectory || selectFolderBtn.disabled) return;
    await loadProject(browseDirectory);
    closeFolderPicker();
  }));

  function sendRawInput(data) {
    if (socket && socket.readyState === WebSocket.OPEN) {
      socket.send(data);
    }
  }

  function sendResize() {
    try {
      fitAddon.fit();
    } catch (_) {
      return;
    }
    if (socket && socket.readyState === WebSocket.OPEN && term.cols && term.rows) {
      socket.send(
        JSON.stringify({
          type: "resize",
          cols: term.cols,
          rows: term.rows,
        })
      );
    }
  }

  function connectWebSocket() {
    if (!activeSession) return;
    if (reconnectTimer) {
      clearTimeout(reconnectTimer);
      reconnectTimer = null;
    }

    detachSocket();
    const epoch = generation;
    const session = activeSession;
    let exited = false;

    setStatus("connecting", "Connecting");
    socket = new WebSocket(buildWsUrl());
    const connection = socket;
    connection.binaryType = "arraybuffer";

    connection.addEventListener("open", () => {
      if (epoch !== generation) return;
      reconnectAttempts = 0;
      term.reset(); // The server replays history on every attachment.
      setStatus("connected", "Live");
      sendResize();
      term.focus();
    });

    connection.addEventListener("message", (event) => {
      if (epoch !== generation) return;
      if (event.data instanceof ArrayBuffer) {
        term.write(new Uint8Array(event.data));
      } else if (typeof event.data === "string") {
        if (event.data.startsWith("{")) {
          try {
            const msg = JSON.parse(event.data);
            if (msg.type === "pong") return;
            if (msg.type === "exit") {
              exited = true;
              session.exited = true;
              setStatus("disconnected", "Exited");
              renderSessions();
              return;
            }
          } catch (_) {
            // Normal terminal output starting with '{'
          }
        }
        term.write(event.data);
      }
    });

    connection.addEventListener("close", (event) => {
      if (epoch !== generation || connection !== socket) return;
      socket = null;
      if (exited || session.exited || event.code === 1000) {
        setStatus("disconnected", "Exited");
        return;
      }
      setStatus("disconnected", "Reconnecting...");
      reconnectTimer = setTimeout(async () => {
        try {
          const result = await api("GET", { project });
          if (epoch !== generation) return;
          if (!result.sessions.some((item) => item.id === session.id)) {
            sessions = result.sessions;
            activateSession(sessions[0] || null);
            showError(new Error("This session is no longer available. Create a new terminal."));
            return;
          }
          connectWebSocket();
        } catch (error) {
          if (epoch !== generation) return;
          showError(error);
          if (++reconnectAttempts < 5) connectWebSocket();
          else setStatus("disconnected", "Offline — reopen project");
        }
      }, Math.min(1000 * 2 ** reconnectAttempts++, 10000));
    });

    connection.addEventListener("error", () => {
      if (epoch !== generation) return;
      setStatus("disconnected", "Offline");
    });
  }

  function updateModifiersUI() {
    if (ctrlToggle) {
      if (isCtrlActive) ctrlToggle.classList.add("active");
      else ctrlToggle.classList.remove("active");
    }
    if (altToggle) {
      if (isAltActive) altToggle.classList.add("active");
      else altToggle.classList.remove("active");
    }
  }

  // Forward user typing directly to PTY with modifier key support (Ctrl / Alt)
  term.onData((data) => {
    if (isCtrlActive) {
      isCtrlActive = false;
      updateModifiersUI();
      if (data.length === 1) {
        const charCode = data.toUpperCase().charCodeAt(0);
        if (charCode >= 65 && charCode <= 90) { // A-Z
          sendRawInput(String.fromCharCode(charCode - 64));
          return;
        } else if (data === " " || data === "@") {
          sendRawInput("\x00"); // Ctrl+Space / Ctrl+@
          return;
        }
      }
    }

    if (isAltActive) {
      isAltActive = false;
      updateModifiersUI();
      sendRawInput("\x1b" + data);
      return;
    }

    sendRawInput(data);
  });

  // Handle viewport and mobile virtual keyboard resizes
  window.addEventListener("resize", () => {
    sendResize();
  });
  if (window.visualViewport) {
    window.visualViewport.addEventListener("resize", () => {
      document.getElementById("app").style.height = `${window.visualViewport.height}px`;
      sendResize();
    });
  }

  // 2. Mobile Touch Keypad Bar Actions
  const KEY_SEQUENCES = {
    esc: "\x1b",
    tab: "\t",
    "ctrl-c": "\x03",
    "ctrl-d": "\x04",
    up: "\x1b[A",
    down: "\x1b[B",
    left: "\x1b[D",
    right: "\x1b[C",
    enter: "\r",
  };

  async function handlePaste() {
    try {
      if (navigator.clipboard && navigator.clipboard.readText) {
        const text = await navigator.clipboard.readText();
        if (text) {
          sendRawInput(text);
          term.focus();
          return;
        }
      }
    } catch (_) {
      // Fallback to prompt drawer if clipboard permission is blocked on HTTP
    }
    togglePromptDrawer(true);
    promptInput.placeholder = "Long-press here and tap Paste, then click Send to Terminal...";
    promptInput.focus();
  }

  document.querySelectorAll(".key-btn").forEach((btn) => {
    btn.addEventListener("click", (e) => {
      e.preventDefault();
      const action = btn.getAttribute("data-action");

      if (action === "toggle-ctrl") {
        isCtrlActive = !isCtrlActive;
        updateModifiersUI();
      } else if (action === "toggle-alt") {
        isAltActive = !isAltActive;
        updateModifiersUI();
      } else if (action === "paste") {
        handlePaste();
      } else if (action === "clear") {
        term.clear();
        sendRawInput("\x0c"); // Ctrl+L
        term.focus();
      } else if (KEY_SEQUENCES[action]) {
        sendRawInput(KEY_SEQUENCES[action]);
        term.focus();
      }
    });
  });

  // 3. Shell Choice Modal Logic
  function openShellModal() {
    shellModal.classList.remove("hidden");
  }

  function closeShellModal() {
    shellModal.classList.add("hidden");
  }

  shellPickerBtn.addEventListener("click", openShellModal);
  shellModalClose.addEventListener("click", closeShellModal);

  document.querySelectorAll(".shell-card").forEach((card) => {
    card.addEventListener("click", () => {
      const selected = card.getAttribute("data-shell");
      closeShellModal();
      runAction(async () => {
        setStatus("connecting", selected === "wsl" ? "Starting WSL…" : "Starting shell…");
        try {
          const session = await api("POST", {}, { shell: selected, project });
          sessions.push(session);
          activateSession(session);
        } catch (error) {
          setStatus(socket?.readyState === WebSocket.OPEN ? "connected" : "disconnected",
            socket?.readyState === WebSocket.OPEN ? "Live" : "Launch failed");
          throw error;
        }
      });
    });
  });

  // 4. Prompt Drawer & Web Speech API Voice-to-Text Integration
  function togglePromptDrawer(forceOpen) {
    const isHidden = promptDrawer.classList.contains("hidden");
    const shouldOpen = typeof forceOpen === "boolean" ? forceOpen : isHidden;
    if (shouldOpen) {
      promptDrawer.classList.remove("hidden");
      promptInput.focus();
    } else {
      promptDrawer.classList.add("hidden");
      term.focus();
    }
  }

  promptModalBtn.addEventListener("click", () => togglePromptDrawer());
  drawerCloseBtn.addEventListener("click", () => togglePromptDrawer(false));

  function sendComposedPrompt() {
    const text = promptInput.value.trim();
    if (!text) return;
    const suffix = autoEnterCheck.checked ? "\r" : "";
    sendRawInput(text + suffix);
    promptInput.value = "";
    togglePromptDrawer(false);
  }

  drawerSendBtn.addEventListener("click", sendComposedPrompt);
  promptInput.addEventListener("keydown", (e) => {
    if (e.key === "Enter" && (e.ctrlKey || e.metaKey)) {
      e.preventDefault();
      sendComposedPrompt();
    }
  });

  const SpeechRecognition = window.SpeechRecognition || window.webkitSpeechRecognition;
  let recognition = null;
  let isListening = false;

  if (SpeechRecognition) {
    recognition = new SpeechRecognition();
    recognition.continuous = false;
    recognition.interimResults = true;
    recognition.lang = navigator.language || "en-US";

    recognition.onstart = () => {
      isListening = true;
      voiceBtn.classList.add("listening");
      voiceBadge.textContent = "Listening...";
      voiceBadge.classList.remove("hidden");
      drawerMicBtn.textContent = "Stop Mic";
    };

    recognition.onresult = (event) => {
      let finalTranscript = "";
      let interimTranscript = "";

      for (let i = event.resultIndex; i < event.results.length; i++) {
        const piece = event.results[i][0].transcript;
        if (event.results[i].isFinal) {
          finalTranscript += piece;
        } else {
          interimTranscript += piece;
        }
      }

      if (interimTranscript) {
        voiceBadge.textContent = interimTranscript.slice(0, 28) + (interimTranscript.length > 28 ? "..." : "");
      }

      if (finalTranscript) {
        const cleaned = finalTranscript.trim();
        if (!promptDrawer.classList.contains("hidden")) {
          promptInput.value = (promptInput.value ? promptInput.value + " " : "") + cleaned;
          promptInput.focus();
        } else {
          const suffix = autoEnterCheck.checked ? "\r" : "";
          sendRawInput(cleaned + suffix);
        }
      }
    };

    recognition.onerror = (event) => {
      voiceBadge.textContent = "Mic error: " + event.error;
      setTimeout(() => {
        if (!isListening) voiceBadge.classList.add("hidden");
      }, 2200);
    };

    recognition.onend = () => {
      isListening = false;
      voiceBtn.classList.remove("listening");
      drawerMicBtn.textContent = "Dictate";
      setTimeout(() => {
        if (!isListening) voiceBadge.classList.add("hidden");
      }, 800);
    };
  }

  function toggleVoiceRecognition() {
    if (!recognition) {
      togglePromptDrawer(true);
      promptInput.placeholder = "Tap your mobile keyboard microphone icon to dictate a prompt...";
      return;
    }
    if (isListening) {
      recognition.stop();
    } else {
      try {
        recognition.start();
      } catch (_) {
        recognition.stop();
      }
    }
  }

  voiceBtn.addEventListener("click", toggleVoiceRecognition);
  drawerMicBtn.addEventListener("click", toggleVoiceRecognition);

  // Start terminal session
  window.addEventListener("load", () => {
    updateShellUI(currentShell);
    sendResize();
    runAction(async () => {
      await loadProject(project);
      if (!sessions.length) {
        const session = await api("POST", {}, { shell: currentShell, project });
        sessions.push(session);
        activateSession(session);
      }
    });
  });
})();
