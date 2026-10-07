const { createServer } = require('./app');

const port = Number(process.env.PORT || 8088);

createServer().listen(port, () => {
  console.log(`loan-eligibility SOAP mock listening on :${port} (POST /calculator.asmx, GET /calculator.asmx?WSDL)`);
});
