package schemas

import "os"

// osGetenv 抽出便于测试替身(e2e 门禁用)。
func osGetenv(key string) string { return os.Getenv(key) }
