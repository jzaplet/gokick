// Package fixtures exercises every collector violation: shapes the scan
// cannot see (positional, dynamic) and marker misuse (trailing, unused,
// reasonless).
package fixtures

var fieldName = "dynamic"

func positional() error {
	return &ValidationError{"positional", "msg"}
}

func dynamic() error {
	return &ValidationError{Field: fieldName, Message: "msg"}
}

func trailing() error {
	return &ValidationError{Field: "trail", Message: "x"} //gkerrf:exempt trailing
}

func unused() error {
	//gkerrf:exempt nothing below needs this
	return nil
}

func reasonless() error {
	//gkerrf:exempt
	return &ValidationError{Field: "covered", Message: "x"}
}
