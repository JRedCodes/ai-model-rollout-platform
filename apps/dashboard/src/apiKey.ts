const STORAGE_KEY = "rollout-platform:api-key";
const CHANGE_EVENT = "rollout-platform:api-key-changed";

export function getApiKey(): string | null {
  return localStorage.getItem(STORAGE_KEY);
}

export function setApiKey(key: string): void {
  localStorage.setItem(STORAGE_KEY, key);
  window.dispatchEvent(new Event(CHANGE_EVENT));
}

export function clearApiKey(): void {
  localStorage.removeItem(STORAGE_KEY);
  window.dispatchEvent(new Event(CHANGE_EVENT));
}

// Lets the App root re-render (dashboard <-> key gate) whenever the key is
// set or cleared, including from deep inside a fetch call on a 401.
export function onApiKeyChange(callback: () => void): () => void {
  window.addEventListener(CHANGE_EVENT, callback);
  return () => window.removeEventListener(CHANGE_EVENT, callback);
}
