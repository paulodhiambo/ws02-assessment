# Loan eligibility SOAP mock

An offline stand-in for the public **DNE Online calculator** SOAP service
(`http://www.dneonline.com/calculator.asmx`), which is the backend
`POST /loans/eligibility` calls.

It implements the same contract (`wsdl/loan-eligibility.wsdl`, the
`Multiply` and `Divide` operations) and the same observable behaviour,
checked against the live service:

| Input                       | Live service / mock result                |
|-----------------------------|-------------------------------------------|
| `Divide(500000, 12)`        | `41667` (half-to-even rounding)           |
| `Divide(5, 2)`              | `2`                                       |
| `Divide(x, 0)`              | HTTP 500, `soap:Server` fault             |
| `intA` > 2,147,483,647      | HTTP 500, `soap:Client` fault             |

Both return the raw .NET fault with a stack trace in `faultstring`, which is
what MI has to hide from API consumers.

## Why a mock as well as the public service?

- CI should not depend on a free public service being up.
- Tests need to trigger SOAP faults on demand.

Switch MI between the two with `LOAN_SOAP_BACKEND_URL` in `.env`:

```bash
LOAN_SOAP_BACKEND_URL=http://www.dneonline.com/calculator.asmx   # public (default)
LOAN_SOAP_BACKEND_URL=http://soap-mock:8088/calculator.asmx      # local mock
```

## Run

```bash
npm start                 # http://localhost:8088/calculator.asmx
npm test
curl 'http://localhost:8088/calculator.asmx?WSDL'
```
