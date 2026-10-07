const http = require('node:http');
const users = require('./users');

// Special ids that simulate backend failure modes, so the gateway's
// timeout and fault handling can be exercised deterministically.
const SLOW_ID = '999';   // responds after SLOW_DELAY_MS (beyond MI's endpoint timeout)
const ERROR_ID = '500';  // responds 500

function createServer({ slowDelayMs = 15000, log = console.log } = {}) {
  return http.createServer((req, res) => {
    const url = new URL(req.url, 'http://localhost');
    // Log the headers we actually received, so the demo can show what the
    // gateway injected (correlation id) and stripped (internal headers).
    log(JSON.stringify({ method: req.method, path: url.pathname, headers: req.headers }));

    const send = (status, body) => {
      res.writeHead(status, {
        'Content-Type': 'application/json; charset=utf-8',
        // Internal headers a real backend might leak; the gateway must strip them.
        'X-Powered-By': 'Express',
        'X-Backend-Node': 'customer-svc-02',
        'Server': 'mock-customer-service/1.0'
      });
      res.end(JSON.stringify(body));
    };

    if (url.pathname === '/health') return send(200, { status: 'UP' });

    const match = url.pathname.match(/^\/users\/([^/]+)$/);
    if (req.method !== 'GET' || !match) return send(404, {});

    const id = match[1];
    if (id === ERROR_ID) return send(500, { message: 'Internal Server Error' });
    if (id === SLOW_ID) {
      const timer = setTimeout(() => send(200, users[0]), slowDelayMs);
      req.on('close', () => clearTimeout(timer));
      return;
    }

    const user = users.find((u) => String(u.id) === id);
    // JSONPlaceholder returns 404 with an empty object for unknown ids.
    return user ? send(200, user) : send(404, {});
  });
}

module.exports = { createServer, SLOW_ID, ERROR_ID };
