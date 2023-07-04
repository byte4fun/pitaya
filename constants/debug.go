package constants

var (
	Debug     = false
	CanPrint  = false
	LogFilter = map[string]bool{}
)

func SetLogFilter(data map[string]bool) {
	tmp := make(map[string]bool, len(data))
	for k, v := range data {
		tmp[k] = v
	}
	LogFilter = tmp
}

func LogCanPrint(route string) bool {
	val, ok := LogFilter[route]
	if !ok {
		return false
	}
	return val
}
