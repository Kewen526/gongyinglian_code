package sqlutil

import "strings"

var likeReplacer = strings.NewReplacer("%", "\\%", "_", "\\_")

func EscapeLike(s string) string {
	return "%" + likeReplacer.Replace(s) + "%"
}
