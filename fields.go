package toml

import (
	"reflect"
	"strings"
	"sync"
)

// How struct fields meet keys. This follows BurntSushi/toml on purpose, so
// that moving a tool to this library does not change what its config file
// means: the `toml` tag names a field, an untagged exported field is named
// by its Go name, and a key that matches no name exactly may match one
// case-insensitively. An untagged embedded struct lends its fields to the
// outer struct, by Go's rules for promoted fields.

type field struct {
	name   string
	index  []int
	tagged bool
}

type structFields struct {
	exact map[string]*field
	list  []*field // in declaration order, for the case-insensitive match
}

// lookup finds the field for a key: by exact name first, then the first
// field whose name matches case-insensitively.
func (s *structFields) lookup(key string) (*field, bool) {
	if f, ok := s.exact[key]; ok {
		return f, true
	}
	for _, f := range s.list {
		if strings.EqualFold(f.name, key) {
			return f, true
		}
	}
	return nil, false
}

var fieldCache sync.Map // reflect.Type → *structFields

func fieldsOf(t reflect.Type) *structFields {
	if s, ok := fieldCache.Load(t); ok {
		return s.(*structFields)
	}
	s, _ := fieldCache.LoadOrStore(t, typeFields(t))
	return s.(*structFields)
}

func typeFields(t reflect.Type) *structFields {
	type candidate struct {
		field
		depth int
	}
	var cands []candidate
	var walk func(t reflect.Type, index []int, depth int, seen map[reflect.Type]bool)
	walk = func(t reflect.Type, index []int, depth int, seen map[reflect.Type]bool) {
		for i := 0; i < t.NumField(); i++ {
			sf := t.Field(i)
			tag := sf.Tag.Get("toml")
			if tag == "-" {
				continue
			}
			name, _, _ := strings.Cut(tag, ",")
			idx := append(index[:len(index):len(index)], i)
			if sf.Anonymous && name == "" {
				ft := sf.Type
				if ft.Kind() == reflect.Pointer {
					ft = ft.Elem()
				}
				if ft.Kind() == reflect.Struct {
					if !seen[ft] {
						seen[ft] = true
						walk(ft, idx, depth+1, seen)
						delete(seen, ft)
					}
					continue
				}
			}
			if !sf.IsExported() {
				continue
			}
			tagged := name != ""
			if !tagged {
				name = sf.Name
			}
			cands = append(cands, candidate{field{name, idx, tagged}, depth})
		}
	}
	walk(t, nil, 0, map[reflect.Type]bool{t: true})

	// Of the fields that share a name, the shallowest wins; at equal depth a
	// tagged field beats untagged ones; anything still tied is ambiguous and
	// no field gets the name, as encoding/json does.
	byName := map[string][]candidate{}
	var order []string
	for _, c := range cands {
		if _, ok := byName[c.name]; !ok {
			order = append(order, c.name)
		}
		byName[c.name] = append(byName[c.name], c)
	}
	s := &structFields{exact: map[string]*field{}}
	for _, name := range order {
		group := byName[name]
		best := []candidate{}
		for _, c := range group {
			switch {
			case len(best) == 0 || c.depth < best[0].depth:
				best = []candidate{c}
			case c.depth == best[0].depth:
				best = append(best, c)
			}
		}
		if len(best) > 1 {
			var tagged []candidate
			for _, c := range best {
				if c.tagged {
					tagged = append(tagged, c)
				}
			}
			best = tagged
		}
		if len(best) != 1 {
			continue
		}
		f := best[0].field
		s.exact[name] = &f
		s.list = append(s.list, &f)
	}
	return s
}
