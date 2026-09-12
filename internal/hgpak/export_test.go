package hgpak

// CachedChunks and TotalChunks expose the chunk cache to the package's external
// test, which asserts that reading one small file does not decompress the whole
// archive (R4.3). They are test-only rather than part of the API: nothing in
// the program should make a decision from cache occupancy.
func CachedChunks(p *File) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.cache.len()
}

func TotalChunks(p *File) int { return len(p.chunkSizes) }
