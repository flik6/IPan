package httpapi

import "log"

// Version 是构建版本，可通过 -ldflags 注入。
var Version = "dev"

func logInfo(format string, args ...any) {
	log.Printf("[INFO] "+format, args...)
}

func logWarn(format string, args ...any) {
	log.Printf("[WARN] "+format, args...)
}
