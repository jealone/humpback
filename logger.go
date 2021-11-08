package humpback

type Logger interface {
	Printf(format string, v ...interface{})
}
