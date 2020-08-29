package common

type Route interface {
	AddToTable() bool
	AddDestWithOrigin(string) error
}
