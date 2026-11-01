const { test } = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const vm = require("node:vm");

test("late socket callbacks cannot reconnect or overwrite a selected terminal", async () => {
  class Element {
    constructor() {
      this.listeners = {};
      this.children = [];
      this.value = "";
      this.style = {};
      this.classList = { add() {}, remove() {}, toggle() {}, contains() { return true; } };
    }
    addEventListener(name, fn) { this.listeners[name] = fn; }
    setAttribute() {}
    append(...items) { this.children.push(...items); }
    replaceChildren() { this.children = []; }
    focus() {}
  }
  const elements = new Map();
  const element = (id) => {
    if (!elements.has(id)) elements.set(id, new Element());
    return elements.get(id);
  };
  const sockets = [];
  class Socket extends Element {
    static OPEN = 1;
    constructor(url) { super(); this.url = url; this.readyState = 1; sockets.push(this); }
    close() { this.readyState = 3; }
    send() {}
    emit(name, event = {}) { this.listeners[name]?.(event); }
  }
  const writes = [];
  class Terminal {
    loadAddon() {}
    open() {}
    onData() {}
    reset() {}
    focus() {}
    write(data) { writes.push(data); }
  }
  const windowEvents = {};
  const timers = new Map();
  let nextTimer = 0;
  const items = [
    { id: "one", shell: "powershell", project: "D:\\project" },
    { id: "two", shell: "gitbash", project: "D:\\project" },
  ];
  const context = {
    window: {
      location: { search: "?token=test", protocol: "http:", host: "localhost" },
      innerWidth: 390,
      Terminal,
      FitAddon: { FitAddon: class { fit() {} } },
      addEventListener: (name, fn) => { windowEvents[name] = fn; },
    },
    document: { getElementById: element, querySelectorAll: () => [], createElement: () => new Element() },
    localStorage: { getItem() { return null; }, setItem() {} },
    navigator: {}, WebSocket: Socket, URLSearchParams, ArrayBuffer, Uint8Array,
    setTimeout: (fn) => { timers.set(++nextTimer, fn); return nextTimer; },
    clearTimeout: (id) => timers.delete(id),
    fetch: async () => ({ ok: true, status: 200, json: async () => ({ project: "D:\\project", sessions: items }) }),
  };
  vm.runInNewContext(fs.readFileSync(require.resolve("./static/app.js"), "utf8"), context);
  const settle = async () => { for (let i = 0; i < 20; i++) await Promise.resolve(); };
  windowEvents.load();
  await settle();
  assert.equal(sockets.length, 1);
  sockets[0].emit("open");
  element("sessionTabs").children[1].children[0].listeners.click();
  assert.equal(sockets.length, 2);
  sockets[1].emit("open");
  // Reproduce the original bug: a close listener from the previous socket runs late.
  sockets[0].emit("close", { code: 1005 });
  sockets[0].emit("error");
  sockets[0].emit("message", { data: "old terminal output" });
  assert.equal(timers.size, 0);
  assert.equal(element("connText").textContent, "Live");
  assert.equal(writes.length, 0);
  // An actual network loss reconnects to the same session, once.
  sockets[1].emit("close", { code: 1006 });
  assert.equal(timers.size, 1);
  const callback = [...timers.values()][0];
  timers.clear();
  await callback();
  await settle();
  assert.equal(sockets.length, 3);
  assert.match(sockets[2].url, /session=two$/);
  sockets[2].emit("open");
  sockets[2].emit("message", { data: '{"type":"exit"}' });
  sockets[2].emit("close", { code: 1000 });
  assert.equal(timers.size, 0);
  assert.equal(element("connText").textContent, "Exited");

  // Browse drives without changing the active project or opening a shell.
  const roots = [{ name: "C:\\", path: "C:\\" }, { name: "D:\\", path: "D:\\" }];
  let finishSlowListing;
  let selectedProject;
  context.fetch = async (url) => {
    const parsed = new URL(url, "http://localhost");
    if (parsed.pathname === "/api/sessions") {
      selectedProject = parsed.searchParams.get("project");
      return { ok: true, status: 200, json: async () => ({ project: selectedProject, sessions: [] }) };
    }
    const path = parsed.searchParams.get("path");
    if (path === "C:\\") {
      return new Promise((resolve) => {
        finishSlowListing = () => resolve({ ok: true, status: 200, json: async () => ({ path, parent: "", roots, directories: [] }) });
      });
    }
    return { ok: true, status: 200, json: async () => ({
      path, parent: "D:\\", roots,
      directories: path === "D:\\" ? [{ name: "another project", path: "D:\\another project" }] : [],
    }) };
  };
  element("browseProjectBtn").listeners.click();
  await settle();
  assert.equal(element("folderPath").value, "D:\\project");
  assert.equal(element("folderDrive").children.length, 2);
  element("folderDrive").value = "C:\\";
  element("folderDrive").listeners.change();
  element("folderDrive").value = "D:\\";
  element("folderDrive").listeners.change();
  await settle();
  finishSlowListing();
  await settle();
  assert.equal(element("folderPath").value, "D:\\");
  element("folderList").children[0].listeners.click();
  await settle();
  assert.equal(sockets.length, 3, "browsing must not attach another terminal");
  element("selectFolderBtn").listeners.click();
  await settle();
  assert.equal(selectedProject, "D:\\another project");
  assert.equal(element("projectPath").value, selectedProject);
});
