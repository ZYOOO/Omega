const { app, BrowserWindow, BrowserView, ipcMain } = require("electron");
const path = require("node:path");

const omegaUrl = process.env.OMEGA_WEB_URL || "http://127.0.0.1:5174/";

let mainWindow;
let previewView;

function layoutPreviewView() {
  if (!mainWindow || !previewView) return;
  const [width, height] = mainWindow.getContentSize();
  previewView.setBounds({
    x: Math.max(360, Math.floor(width * 0.42)),
    y: 72,
    width: Math.max(360, width - Math.max(360, Math.floor(width * 0.42)) - 18),
    height: Math.max(360, height - 90),
  });
  previewView.setAutoResize({ width: true, height: true });
}

function ensurePreviewView() {
  if (!mainWindow || previewView) return previewView;
  previewView = new BrowserView({
    webPreferences: {
      preload: path.join(__dirname, "preview-preload.cjs"),
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: false,
    },
  });
  mainWindow.setBrowserView(previewView);
  layoutPreviewView();
  return previewView;
}

function createWindow() {
  mainWindow = new BrowserWindow({
    width: 1440,
    height: 960,
    minWidth: 1100,
    minHeight: 720,
    title: "Omega",
    webPreferences: {
      preload: path.join(__dirname, "omega-preload.cjs"),
      contextIsolation: true,
      nodeIntegration: false,
    },
  });

  mainWindow.loadURL(omegaUrl);
  mainWindow.on("resize", layoutPreviewView);
  mainWindow.on("closed", () => {
    mainWindow = null;
    previewView = null;
  });
}

ipcMain.handle("omega-preview:open", async (_event, url) => {
  if (!url || typeof url !== "string") return { ok: false, error: "preview url is required" };
  const view = ensurePreviewView();
  await view.webContents.loadURL(url);
  return { ok: true, url };
});

ipcMain.handle("omega-preview:reload", async () => {
  if (!previewView) return { ok: false, error: "preview is not open" };
  previewView.webContents.reloadIgnoringCache();
  return { ok: true };
});

ipcMain.on("omega-preview:selection", (_event, payload) => {
  if (!mainWindow) return;
  mainWindow.webContents.send("omega-preview:selection", payload);
});

app.whenReady().then(createWindow);

app.on("window-all-closed", () => {
  if (process.platform !== "darwin") app.quit();
});

app.on("activate", () => {
  if (BrowserWindow.getAllWindows().length === 0) createWindow();
});
