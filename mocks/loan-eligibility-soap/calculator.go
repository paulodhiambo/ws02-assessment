package main

// Arithmetic with the same observable behaviour as the .NET service behind
// http://www.dneonline.com/calculator.asmx (verified against it):
//   - inputs are Int32; out-of-range values fail deserialisation (soap:Client)
//   - Divide rounds half-to-even (5/2 = 2, 7/2 = 4)
//   - Divide by zero and Int32 overflow raise soap:Server faults

import (
	"math"
	"regexp"
	"strconv"
	"strings"
)

type soapFault struct {
	Code    string // "soap:Client" | "soap:Server"
	Message string
}

func (f *soapFault) Error() string { return f.Code + ": " + f.Message }

func clientFault(msg string) *soapFault { return &soapFault{"soap:Client", msg} }
func serverFault(msg string) *soapFault { return &soapFault{"soap:Server", msg} }

var integer = regexp.MustCompile(`^\s*-?\d+\s*$`)

func parseInt32(raw *string, name string) (int64, error) {
	if raw == nil {
		return 0, clientFault("Server was unable to read request. ---> Missing element " + name + ".")
	}
	if !integer.MatchString(*raw) {
		return 0, clientFault("Server was unable to read request. ---> Input string was not in a correct format.")
	}
	n, err := strconv.ParseInt(strings.TrimSpace(*raw), 10, 64)
	if err != nil || n < math.MinInt32 || n > math.MaxInt32 {
		return 0, clientFault("Server was unable to read request. ---> Value was either too large or too small for an Int32.")
	}
	return n, nil
}

func checkRange(n int64) (int64, error) {
	if n < math.MinInt32 || n > math.MaxInt32 {
		return 0, serverFault("Server was unable to process request. ---> Arithmetic operation resulted in an overflow.")
	}
	return n, nil
}

// Operations supported by the mock, keyed by SOAP operation name.
var operations = map[string]func(a, b int64) (int64, error){
	"Multiply": func(a, b int64) (int64, error) { return checkRange(a * b) },
	"Divide": func(a, b int64) (int64, error) {
		if b == 0 {
			return 0, serverFault("Server was unable to process request. ---> Arithmetic operation resulted in an overflow.")
		}
		return checkRange(int64(math.RoundToEven(float64(a) / float64(b))))
	},
}
