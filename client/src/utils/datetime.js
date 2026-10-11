// Several backend handlers emit bare UTC "YYYY-MM-DD HH:MM:SS" strings
// (stats last_sync, program last_seen); V8 parses those as local time and
// Safari rejects them. Go time.Time fields arrive RFC3339 and pass through.
const BARE_UTC = /^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}(\.\d+)?$/;

export function formatDateTime(value, fallback = '-') {
  if (!value) return fallback;
  const iso = BARE_UTC.test(value) ? `${value.replace(' ', 'T')}Z` : value;
  const dt = new Date(iso);
  return Number.isNaN(dt.getTime()) ? value : dt.toLocaleString();
}

export default formatDateTime;
