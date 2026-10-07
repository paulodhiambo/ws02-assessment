const { createServer } = require('./app');

const port = Number(process.env.PORT || 3000);
const slowDelayMs = Number(process.env.SLOW_DELAY_MS || 15000);

createServer({ slowDelayMs }).listen(port, () => {
  console.log(`customer-service mock listening on :${port}`);
});
