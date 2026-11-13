// A minimal static file server for the Playwright example site (no real backend — the test mocks the
// /public/v1 calls at the network layer). Serves e2e/fixtures/ at the root and the built SDK bundle at
// /pdpa-cookie-sdk.js, exactly how a tenant would embed it on their own site (CON-01's own install snippet).
import { createReadStream, existsSync } from 'node:fs';
import { createServer } from 'node:http';
import { extname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const here = fileURLToPath(new URL('.', import.meta.url));
const fixtures = join(here, 'fixtures');
const dist = join(here, '..', 'dist');

const types = { '.html': 'text/html', '.js': 'application/javascript' };

createServer((req, res) => {
  const path = req.url === '/' ? '/index.html' : req.url;
  const file = path === '/pdpa-cookie-sdk.js' ? join(dist, 'pdpa-cookie-sdk.js') : join(fixtures, path.slice(1));
  if (!existsSync(file)) {
    res.writeHead(404).end('not found');
    return;
  }
  res.writeHead(200, { 'Content-Type': types[extname(file)] ?? 'application/octet-stream' });
  createReadStream(file).pipe(res);
}).listen(4310, () => console.log('CON-03 example site on http://localhost:4310'));
