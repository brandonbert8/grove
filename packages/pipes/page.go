package pipes

import (
	"github.com/brandonbert8/grove/packages/router"
)

// Page is validated pagination: 1-based Page plus Limit, with the
// derived Offset for queries. Build it with ParsePage, never by hand:
// the constructor enforces bounds so handlers cannot forget.
type Page struct {
	// Page is the 1-based page number.
	Page int
	// Limit is the clamped page size.
	Limit int
	// Offset is (Page-1)*Limit, ready for SQL/OFFSET or slices.
	Offset int
}

// DefaultPageLimit applies when ?limit is absent; DefaultPageMax caps it.
const (
	DefaultPageLimit = 20
	DefaultPageMax   = 100
)

// ParsePage reads ?page (default 1) and ?limit (default DefaultPageLimit,
// capped at max; max <= 0 means DefaultPageMax). Malformed values are
// 400 *grove.HttpError via Query, so handlers stay branch-free:
//
//	p, err := pipes.ParsePage(c, 100)
//	if err != nil {
//	    return err
//	}
//	users := svc.List(p.Limit, p.Offset)
func ParsePage(c router.Context, max int) (Page, error) {
	if max <= 0 {
		max = DefaultPageMax
	}
	page, err := Query(c, "page", 1)
	if err != nil {
		return Page{}, err
	}
	limit, err := Query(c, "limit", DefaultPageLimit)
	if err != nil {
		return Page{}, err
	}
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 1
	}
	if limit > max {
		limit = max
	}
	return Page{Page: page, Limit: limit, Offset: (page - 1) * limit}, nil
}
