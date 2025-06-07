package bencode

import (
	"reflect"
	"slices"
	"strings"
	"sync"
)

const (
	bencodeTagName = "bencode"
)

var (
	// structInfoCache caches metadata for struct types.
	structInfoCache      = make(map[reflect.Type][]cachedStructFieldInfo)
	structInfoCacheMutex sync.RWMutex
)

// cachedStructFieldInfo holds pre-calculated information about a struct field.
type cachedStructFieldInfo struct {
	fieldName  string
	bencodeTag string
	index      int
	typ        reflect.Type
}

// getCachedStructInfo retrieves or computes and caches metadata for a struct type.
func getCachedStructInfo(typ reflect.Type) []cachedStructFieldInfo {
	structInfoCacheMutex.RLock()
	info, found := structInfoCache[typ]
	structInfoCacheMutex.RUnlock()
	if found {
		return info
	}

	structInfoCacheMutex.Lock()
	defer structInfoCacheMutex.Unlock()
	// Double-check in case another goroutine populated it while waiting for the lock.
	if info, found = structInfoCache[typ]; found {
		return info
	}

	var fields []cachedStructFieldInfo
	for i := range typ.NumField() {
		field := typ.Field(i)
		if !field.IsExported() {
			continue
		}

		bencodeName := field.Tag.Get(bencodeTagName)

		if bencodeName == "" {
			// If no tag is specified, use the field name as the bencode tag.
			bencodeName = field.Name
		}

		fields = append(fields, cachedStructFieldInfo{
			fieldName:  field.Name,
			bencodeTag: bencodeName,
			index:      i,
			typ:        field.Type,
		})
	}

	slices.SortFunc(fields, func(a, b cachedStructFieldInfo) int {
		return strings.Compare(a.bencodeTag, b.bencodeTag)
	})

	structInfoCache[typ] = fields
	return fields
}

func ClearStructInfoCache() {
	structInfoCacheMutex.Lock()
	defer structInfoCacheMutex.Unlock()
	structInfoCache = make(map[reflect.Type][]cachedStructFieldInfo)
}
