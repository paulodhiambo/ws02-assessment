const test = require('node:test');
const assert = require('node:assert');
const { createServer } = require('./app');
const { roundHalfEven } = require('./calculator');

const request = (op, a, b) =>
  '<soap:Envelope xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/"><soap:Body>' +
  `<${op} xmlns="http://tempuri.org/"><intA>${a}</intA><intB>${b}</intB></${op}>` +
  '</soap:Body></soap:Envelope>';

async function call(op, a, b) {
  const server = createServer({ log: () => {} }).listen(0);
  await new Promise((r) => server.once('listening', r));
  try {
    const res = await fetch(`http://127.0.0.1:${server.address().port}/calculator.asmx`, {
      method: 'POST',
      headers: { 'Content-Type': 'text/xml; charset=utf-8', SOAPAction: `"http://tempuri.org/${op}"` },
      body: request(op, a, b)
    });
    return { status: res.status, body: await res.text() };
  } finally {
    server.close();
  }
}

test('rounds half to even like the .NET service', () => {
  assert.strictEqual(roundHalfEven(2.5), 2);
  assert.strictEqual(roundHalfEven(3.5), 4);
  assert.strictEqual(roundHalfEven(41666.67), 41667);
});

test('Divide returns a SOAP response', async () => {
  const { status, body } = await call('Divide', 500000, 12);
  assert.strictEqual(status, 200);
  assert.match(body, /<DivideResult>41667<\/DivideResult>/);
});

test('Int32 overflow is a soap:Client fault', async () => {
  const { status, body } = await call('Divide', 3000000000, 12);
  assert.strictEqual(status, 500);
  assert.match(body, /<faultcode>soap:Client<\/faultcode>/);
});

test('divide by zero is a soap:Server fault', async () => {
  const { status, body } = await call('Divide', 10, 0);
  assert.strictEqual(status, 500);
  assert.match(body, /<faultcode>soap:Server<\/faultcode>/);
});
