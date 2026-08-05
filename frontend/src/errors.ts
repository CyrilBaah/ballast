// Translates known technical error substrings into plain-language copy a
// non-technical user can understand (FR-003), with a generic fallback for
// anything unmapped. Order matters -- the first matching substring wins.
const KNOWN_ERROR_PATTERNS: Array<{ substring: string; message: string }> = [
    { substring: 'access_denied', message: 'Sign-in was cancelled.' },
    {
        substring: 'secure credential storage',
        message: "Your system's secure credential storage is unavailable, so Ballast can't sign in safely right now.",
    },
    { substring: 'network', message: "Couldn't connect. Check your internet connection and try again." },
    { substring: 'connection', message: "Couldn't connect. Check your internet connection and try again." },
    { substring: 'storagequotaexceeded', message: 'Your Google Drive storage is full.' },
    { substring: 'ratelimitexceeded', message: 'Google Drive is busy right now. Please try again in a moment.' },
    { substring: 'notfound', message: "That destination folder is no longer available." },
    { substring: 'timed out', message: 'That took too long and timed out. Please try again.' },
];

const GENERIC_FALLBACK = 'Something went wrong. Please try again.';

// toPlainLanguage maps a raw error/failure message to user-facing copy.
// Falls back to the raw message if it already reads as plain language
// (short, no obvious technical markers), otherwise a generic fallback.
export function toPlainLanguage(rawMessage: string): string {
    const lower = rawMessage.toLowerCase();
    for (const { substring, message } of KNOWN_ERROR_PATTERNS) {
        if (lower.includes(substring)) return message;
    }
    // Anything containing classic technical markers but not otherwise
    // recognized falls back to the generic message rather than leaking
    // Go error wrapping ("upload: ...: %w") or raw JSON to the user.
    if (/:\s|error|failed|exception|\bstatus \d{3}\b/i.test(rawMessage) && rawMessage.length > 60) {
        return GENERIC_FALLBACK;
    }
    return rawMessage || GENERIC_FALLBACK;
}
