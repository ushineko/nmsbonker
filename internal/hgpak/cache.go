package hgpak

import "container/list"

/*
chunkCache is a fixed-size LRU of decompressed chunks (R4.3).

Without it, reading a hundred small MBINs out of one table pak decompresses the
same 64 KiB chunk a hundred times, because the files that live near each other
in the pak are exactly the ones a build reads together. With it, a build reads
each chunk about once. The cache is per open archive and holds decompressed
bytes, so its cost is bounded and predictable: capacity x 64 KiB.

Not sync.Map or an off-the-shelf LRU: the caller already holds the archive's
mutex when it gets here (the file handle, the decoder and the cache are touched
as one operation), so a second layer of locking would only add contention.
*/
type chunkCache struct {
	capacity int
	order    *list.List
	items    map[int]*list.Element
}

type cacheEntry struct {
	index int
	data  []byte
}

func newChunkCache(capacity int) *chunkCache {
	if capacity < 1 {
		capacity = 1
	}
	return &chunkCache{
		capacity: capacity,
		order:    list.New(),
		items:    make(map[int]*list.Element, capacity),
	}
}

func (c *chunkCache) get(index int) ([]byte, bool) {
	el, ok := c.items[index]
	if !ok {
		return nil, false
	}
	c.order.MoveToFront(el)
	return el.Value.(*cacheEntry).data, true
}

func (c *chunkCache) put(index int, data []byte) {
	if el, ok := c.items[index]; ok {
		el.Value.(*cacheEntry).data = data
		c.order.MoveToFront(el)
		return
	}
	c.items[index] = c.order.PushFront(&cacheEntry{index: index, data: data})
	for c.order.Len() > c.capacity {
		oldest := c.order.Back()
		if oldest == nil {
			return
		}
		c.order.Remove(oldest)
		delete(c.items, oldest.Value.(*cacheEntry).index)
	}
}

// reset drops everything, so closing an archive releases its chunks.
func (c *chunkCache) reset() {
	c.order.Init()
	c.items = map[int]*list.Element{}
}

// len reports how many chunks are held. Used by the test that proves reading
// one small file does not decompress a whole pak (R4.3).
func (c *chunkCache) len() int { return c.order.Len() }
