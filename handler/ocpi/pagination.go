package ocpi

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"cposim/ocpi"
)

const (
	defaultPageLimit = 50
	maxPageLimit     = 100
)

type pageParams struct {
	dateFrom time.Time
	dateTo   time.Time
	limit    int
	offset   int
}

// paginator selects one page and reports how many items matched the date filter in total.
type paginator func(params pageParams) (items any, totalCount int)

// pageOf implements OCPI list semantics: filter on last_updated in [date_from, date_to), then
// apply offset and limit.
func pageOf[T any](all []T, lastUpdated func(T) time.Time) paginator {
	return func(params pageParams) (any, int) {
		matching := []T{}
		for _, item := range all {
			updatedAt := lastUpdated(item)
			if !params.dateFrom.IsZero() && updatedAt.Before(params.dateFrom) {
				continue
			}

			if !params.dateTo.IsZero() && !updatedAt.Before(params.dateTo) {
				continue
			}

			matching = append(matching, item)
		}

		start := min(params.offset, len(matching))
		end := min(start+params.limit, len(matching))

		return matching[start:end], len(matching)
	}
}

func (h handler) respondPage(w http.ResponseWriter, r *http.Request, paginate paginator) {
	params, err := parsePageParams(r.URL.Query())
	if err != nil {
		h.respond(w, http.StatusBadRequest, ocpi.StatusCodeInvalidParams, err.Error(), nil)
		return
	}

	items, totalCount := paginate(params)

	w.Header().Set("X-Limit", strconv.Itoa(params.limit))
	w.Header().Set("X-Total-Count", strconv.Itoa(totalCount))

	if nextOffset := params.offset + params.limit; nextOffset < totalCount {
		query := r.URL.Query()
		query.Set("limit", strconv.Itoa(params.limit))
		query.Set("offset", strconv.Itoa(nextOffset))
		w.Header().Set("Link", fmt.Sprintf(`<%s%s?%s>; rel="next"`, baseURL(r), r.URL.Path, query.Encode()))
	}

	h.respond(w, http.StatusOK, ocpi.StatusCodeSuccess, "", items)
}

func parsePageParams(query url.Values) (pageParams, error) {
	params := pageParams{limit: defaultPageLimit}

	var err error
	if params.dateFrom, err = parseOptionalTime(query.Get("date_from")); err != nil {
		return pageParams{}, fmt.Errorf("date_from is not a valid timestamp: %q", query.Get("date_from"))
	}

	if params.dateTo, err = parseOptionalTime(query.Get("date_to")); err != nil {
		return pageParams{}, fmt.Errorf("date_to is not a valid timestamp: %q", query.Get("date_to"))
	}

	if text := query.Get("offset"); text != "" {
		if params.offset, err = strconv.Atoi(text); err != nil || params.offset < 0 {
			return pageParams{}, fmt.Errorf("offset is not a non-negative integer: %q", text)
		}
	}

	if text := query.Get("limit"); text != "" {
		if params.limit, err = strconv.Atoi(text); err != nil || params.limit <= 0 {
			return pageParams{}, fmt.Errorf("limit is not a positive integer: %q", text)
		}
	}

	params.limit = min(params.limit, maxPageLimit)

	return params, nil
}

func parseOptionalTime(text string) (time.Time, error) {
	if text == "" {
		return time.Time{}, nil
	}

	return time.Parse(time.RFC3339, text)
}
