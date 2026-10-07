const http = require('node:http');
const fs = require('node:fs');
const path = require('node:path');
const { operations, parseInt32, SoapFault } = require('./calculator');

const WSDL_PATH = process.env.WSDL_PATH || path.join(__dirname, '..', 'wsdl', 'loan-eligibility.wsdl');

const envelope = (body) =>
  '<?xml version="1.0" encoding="utf-8"?>' +
  '<soap:Envelope xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/" ' +
  'xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xmlns:xsd="http://www.w3.org/2001/XMLSchema">' +
  `<soap:Body>${body}</soap:Body></soap:Envelope>`;

const escapeXml = (s) => s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');

// Like the real .NET service, the fault string includes a (fake) stack trace:
// the gateway must never pass this through to API consumers.
const faultXml = (fault) => envelope(
  `<soap:Fault><faultcode>${fault.code}</faultcode>` +
  `<faultstring>${escapeXml(`System.Web.Services.Protocols.SoapException: ${fault.message}\n   at Calculator.Invoke() in C:\\inetpub\\calculator\\Calculator.asmx.cs:line 42`)}</faultstring>` +
  '<detail /></soap:Fault>'
);

// Extracts <Op xmlns="http://tempuri.org/"><intA>..</intA><intB>..</intB></Op>
// from the body regardless of namespace prefix. Good enough for a mock.
function parseRequest(xml) {
  const op = Object.keys(operations).find((name) =>
    new RegExp(`<(?:\\w+:)?${name}[\\s>]`).test(xml));
  const field = (name) => {
    const m = xml.match(new RegExp(`<(?:\\w+:)?${name}>([^<]*)</(?:\\w+:)?${name}>`));
    return m ? m[1] : undefined;
  };
  return { op, intA: field('intA'), intB: field('intB') };
}

function createServer({ log = console.log } = {}) {
  return http.createServer((req, res) => {
    const url = new URL(req.url, 'http://localhost');

    if (url.pathname === '/health') {
      res.writeHead(200, { 'Content-Type': 'application/json' });
      return res.end('{"status":"UP"}');
    }

    if (req.method === 'GET' && url.pathname === '/calculator.asmx' && /^wsdl$/i.test(url.search.slice(1))) {
      res.writeHead(200, { 'Content-Type': 'text/xml; charset=utf-8' });
      return fs.createReadStream(WSDL_PATH).pipe(res);
    }

    if (req.method !== 'POST' || url.pathname !== '/calculator.asmx') {
      res.writeHead(404);
      return res.end();
    }

    let body = '';
    req.on('data', (c) => { body += c; });
    req.on('end', () => {
      const { op, intA, intB } = parseRequest(body);
      log(JSON.stringify({ soapAction: req.headers.soapaction, op, intA, intB }));
      try {
        if (!op) throw new SoapFault('soap:Client', 'Server did not recognize the value of HTTP Header SOAPAction.');
        const result = operations[op](parseInt32(intA, 'intA'), parseInt32(intB, 'intB'));
        res.writeHead(200, { 'Content-Type': 'text/xml; charset=utf-8' });
        res.end(envelope(`<${op}Response xmlns="http://tempuri.org/"><${op}Result>${result}</${op}Result></${op}Response>`));
      } catch (err) {
        const fault = err instanceof SoapFault ? err : new SoapFault('soap:Server', err.message);
        res.writeHead(500, { 'Content-Type': 'text/xml; charset=utf-8' });
        res.end(faultXml(fault));
      }
    });
  });
}

module.exports = { createServer, parseRequest };
