const { contextBridge, ipcRenderer } = require("electron");

contextBridge.exposeInMainWorld("omegaDesktop", {
  getServices: () => ipcRenderer.invoke("omega-desktop:services"),
  reloadApp: () => ipcRenderer.invoke("omega-app:reload"),
  resolvePreviewTarget: (target) => ipcRenderer.invoke("omega-preview:resolve-target", target),
  startPreviewDevServer: (input) => ipcRenderer.invoke("omega-preview:start-dev-server", input),
  openPreview: (url) => ipcRenderer.invoke("omega-preview:open", url),
  reloadPreview: (input) => ipcRenderer.invoke("omega-preview:reload", input),
  closePreview: () => ipcRenderer.invoke("omega-preview:close"),
  startPreviewSelection: () => ipcRenderer.invoke("omega-preview:start-selection"),
  onPreviewSelection: (handler) => {
    const listener = (_event, payload) => handler(payload);
    ipcRenderer.on("omega-preview:selection", listener);
    return () => ipcRenderer.removeListener("omega-preview:selection", listener);
  },
});
