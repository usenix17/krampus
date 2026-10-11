package handlers

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

type pageParams struct {
	Page    int
	PerPage int
	Search  string
	Offset  int
}

type pageResponse struct {
	Items   any `json:"items"`
	Total   int `json:"total"`
	Page    int `json:"page"`
	PerPage int `json:"per_page"`
}

func getPageParams(c *gin.Context) pageParams {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "25"))
	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 200 {
		perPage = 25
	}
	return pageParams{Page: page, PerPage: perPage,
		Search: c.Query("search"), Offset: (page - 1) * perPage}
}

// likePattern escapes LIKE wildcards in user input and wraps it for a
// contains match. Always bind the result as a parameter with ESCAPE '\'.
func likePattern(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return "%" + s + "%"
}

// searchClause builds an ORed `col LIKE ? ESCAPE '\'` fragment over cols for
// a non-empty search term, returning the SQL and one bound arg per column.
func searchClause(search string, cols ...string) (string, []any) {
	if search == "" {
		return "", nil
	}
	like := likePattern(search)
	conds := make([]string, len(cols))
	args := make([]any, len(cols))
	for i, col := range cols {
		conds[i] = col + ` LIKE ? ESCAPE '\'`
		args[i] = like
	}
	return "(" + strings.Join(conds, " OR ") + ")", args
}
