/// <reference types="vite/client" />

interface ImportMetaEnv {
  readonly VITE_GITHUB_CLIENT_ID?: string;
  readonly VITE_LINEAR_CLIENT_ID?: string;
  readonly VITE_GOOGLE_CLIENT_ID?: string;
  readonly VITE_MISSION_CONTROL_API_URL?: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}
