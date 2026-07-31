export const VERSION = import.meta.env.VITE_APP_VERSION || "0.0.0";
export const DISPLAY_VERSION = /^v/i.test(VERSION) ? VERSION : `v${VERSION}`;
export const SOURCE_URL = "https://github.com/lwmacct/260607-ociget";
