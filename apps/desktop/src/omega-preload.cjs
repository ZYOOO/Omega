const { contextBridge, ipcRenderer } = require("electron");

contextBridge.exposeInMainWorld("omegaDesktop", {
  openPreview: (url) => ipcRenderer.invoke("omega-preview:open", url),
  reloadPreview: () => ipcRenderer.invoke("omega-preview:reload"),
  onPreviewSelection: (handler) => {
    const listener = (_event, payload) => handler(payload);
    ipcRenderer.on("omega-preview:selection", listener);
    return () => ipcRenderer.removeListener("omega-preview:selection", listener);
  },
});
