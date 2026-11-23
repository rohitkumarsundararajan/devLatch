package diff

import "github.com/rohitkumarsundararajan/devlatch/internal/infer"

// EndpointMissing is reported by probe when a contract endpoint was not
// observed (status code or content type changed, or the request failed).
const EndpointMissing ViolationType = "ENDPOINT_MISSING"

// MissingEndpoints returns a BREAKING violation for each expected endpoint key
// that is absent from observed.
func MissingEndpoints(expected []string, observed map[string]*infer.Node) []Violation {
	var out []Violation
	for _, k := range expected {
		if _, ok := observed[k]; !ok {
			out = append(out, Violation{
				Endpoint: k,
				Path:     "$",
				Type:     EndpointMissing,
				Severity: SeverityBreaking,
				Evidence: []string{"endpoint not observed: status code or content type changed, or the request failed"},
			})
		}
	}
	SortViolations(out)
	return out
}
