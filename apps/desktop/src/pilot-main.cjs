const { app, BrowserWindow, shell } = require("electron");
const path = require("node:path");

const targetUrl = process.env.OMEGA_PAGE_PILOT_URL || "http://127.0.0.1:5173/";

function createWindow() {
  const window = new BrowserWindow({
    width: 1440,
    height: 960,
    minWidth: 960,
    minHeight: 680,
    title: "Omega Page Pilot",
    webPreferences: {
      preload: path.join(__dirname, "pilot-preload.cjs"),
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: false,
    },
  });

  window.webContents.setWindowOpenHandler(({ url }) => {
    shell.openExternal(url);
    return { action: "deny" };
  });
  window.loadURL(targetUrl);
}

app.whenReady().then(createWindow);

app.on("window-all-closed", () => {
  if (process.platform !== "darwin") app.quit();
});

app.on("activate", () => {
  if (BrowserWindow.getAllWindows().length === 0) createWindow();
});
