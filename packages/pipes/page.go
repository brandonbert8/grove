package pipes

import (
	"fmt"
	"math"

	grove "github.com/brandonbert8/grove/packages/core"
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
// 400 *grove.HttpError via Query, so handlers stay branch-free.
// Out-of-range values clamp (documented): page<1→1, limit<1→1,
// limit>max→max. Offsets beyond maxOffset fail with 400 to protect
// the database from giant skips:
//
//	p, err := pipes.ParsePage(c, 100)
//	if err != nil {
//	    return err
//	}
//	users := svc.List(p.Limit, p.Offset)
//
// maxOffset bounds (page-1)*limit (default 10_000); <=0 keeps default.
func ParsePage(c router.Context, max int) (Page, error) {
	return ParsePageWithMaxOffset(c, max, 10_000)
}

// ParsePageWithMaxOffset is ParsePage with an explicit offset cap.
func ParsePageWithMaxOffset(c router.Context, max, maxOffset int) (Page, error) {
	if max <= 0 {
		max = DefaultPageMax
	}
	if maxOffset <= 0 {
		maxOffset = 10_000
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
	// Overflow-safe offset: (page-1)*limit in 64-bit, then bound.
	offset64 := int64(page-1) * int64(limit)
	if offset64 > int64(maxOffset) {
		return Page{}, grove.BadRequest(fmt.Sprintf("page offset %d exceeds maximum %d", offset64, maxOffset))
	}
	if offset64 > int64(math.MaxInt) {
		return Page{}, grove.BadRequest("page offset overflows")
	}
	return Page{Page: page, Limit: limit, Offset: int(offset64)}, nil
}
