package storage

// GroupTagRows groups raw tag rows by event_id and decodes full_json for each group.
// Input rows must be sorted by (event_id, pos) for correct tag ordering.
func GroupTagRows(rows []EventTagRow) (map[string][][]string, error) {
	result := make(map[string][][]string)
	for _, tr := range rows {
		parts, err := DecodeTagFullJSON(tr.FullJSON)
		if err != nil {
			return nil, err
		}
		result[tr.EventID] = append(result[tr.EventID], parts)
	}
	return result, nil
}
