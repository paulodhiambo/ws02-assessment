// Arithmetic with the same observable behaviour as the .NET service behind
// http://www.dneonline.com/calculator.asmx (verified against it):
//   - inputs are Int32; out-of-range values fail deserialisation (soap:Client)
//   - Divide rounds half-to-even (5/2 = 2, 7/2 = 4)
//   - Divide by zero and Int32 overflow raise soap:Server faults

const INT32_MIN = -2147483648;
const INT32_MAX = 2147483647;

class SoapFault extends Error {
  constructor(code, message) {
    super(message);
    this.code = code; // 'soap:Client' | 'soap:Server'
  }
}

function parseInt32(raw, name) {
  if (raw === undefined) throw new SoapFault('soap:Client', `Server was unable to read request. ---> Missing element ${name}.`);
  if (!/^\s*-?\d+\s*$/.test(raw)) {
    throw new SoapFault('soap:Client', 'Server was unable to read request. ---> Input string was not in a correct format.');
  }
  const n = Number(raw);
  if (n < INT32_MIN || n > INT32_MAX) {
    throw new SoapFault('soap:Client', 'Server was unable to read request. ---> Value was either too large or too small for an Int32.');
  }
  return n;
}

function roundHalfEven(x) {
  const floor = Math.floor(x);
  const diff = x - floor;
  if (diff > 0.5) return floor + 1;
  if (diff < 0.5) return floor;
  return floor % 2 === 0 ? floor : floor + 1;
}

function checkRange(n) {
  if (n < INT32_MIN || n > INT32_MAX) {
    throw new SoapFault('soap:Server', 'Server was unable to process request. ---> Arithmetic operation resulted in an overflow.');
  }
  return n;
}

const operations = {
  Multiply: (a, b) => checkRange(a * b),
  Divide: (a, b) => {
    if (b === 0) throw new SoapFault('soap:Server', 'Server was unable to process request. ---> Arithmetic operation resulted in an overflow.');
    return checkRange(roundHalfEven(a / b));
  }
};

module.exports = { operations, parseInt32, roundHalfEven, SoapFault };
