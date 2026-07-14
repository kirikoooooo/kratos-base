package file

import "kratos-demo/internal/data/common"

func FormatUnifiedDiff(path, before, after string) string {
	return common.FormatUnifiedDiff(path, before, after)
}
