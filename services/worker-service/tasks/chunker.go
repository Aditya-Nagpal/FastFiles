package tasks

func chunkText(text string, chunkSize, overlap int) []string {
	runes := []rune(text)
	var chunks []string

	if len(text) <= chunkSize {
		return []string{text}
	}

	start := 0
	for start < len(runes) {
		end := start + chunkSize
		if end > len(runes) {
			end = len(runes)
		}

		chunks = append(chunks, string(runes[start:end]))

		// Move start point, but subtract overlap to keep context
		start += (chunkSize - overlap)

		// Safety break if we aren't moving forward
		if start >= len(runes) || chunkSize <= overlap {
			break
		}
	}
	return chunks
}
