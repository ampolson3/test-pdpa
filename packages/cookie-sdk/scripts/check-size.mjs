// Fails the build if the bundled SDK exceeds CON-01's own acceptance criterion: "ขนาด SDK ไม่เกิน 30 KB gzip".
import { gzipSync } from 'node:zlib';
import { readFileSync } from 'node:fs';

const LIMIT_BYTES = 30 * 1024;
const path = new URL('../dist/pdpa-cookie-sdk.js', import.meta.url);
const raw = readFileSync(path);
const gzipped = gzipSync(raw, { level: 9 });

console.log(`pdpa-cookie-sdk.js: ${raw.length} bytes raw, ${gzipped.length} bytes gzip (limit ${LIMIT_BYTES})`);
if (gzipped.length > LIMIT_BYTES) {
  console.error(`FAIL: bundle exceeds the 30 KB gzip budget by ${gzipped.length - LIMIT_BYTES} bytes`);
  process.exit(1);
}
