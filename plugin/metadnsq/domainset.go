package metadnsq

import (
	"fmt"
	"strings"

	"github.com/coredns/coredns/plugin"
	"golang.org/x/net/idna"
)

// XXX: not thread safe
type StringSet map[string]struct{}

func (s *StringSet) Add(str string) {
	(*s)[str] = struct{}{}
}

func (s *StringSet) Contains(str string) bool {
	if s == nil {
		return false
	}
	_, found := (*s)[str]
	return found
}

// uint16 used to store first two ASCII characters
type domainSet map[uint16]StringSet

func (d domainSet) String() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%T[", d))

	var i uint64
	n := d.Len()
	for _, s := range d {
		for name := range s {
			sb.WriteString(name)
			if i++; i != n {
				sb.WriteString(", ")
			}
		}
	}
	sb.WriteString("]")

	return sb.String()
}

// Return total number of domains in the domain set
func (d *domainSet) Len() uint64 {
	var n uint64
	for _, s := range *d {
		n += uint64(len(s))
	}
	return n
}

func domainToIndex(s string) uint16 {
	n := len(s)
	if n == 0 {
		panic(fmt.Sprintf("Unexpected empty string?!"))
	}
	// Since we use two ASCII characters to present index
	//	Insufficient length will padded with '-'
	//	Since a valid domain segment will never begin with '-'
	//	So it can maintain balance between buckets
	if n == 1 {
		return (uint16('-') << 8) | uint16(s[0])
	}
	// The index will be encoded in big endian
	return (uint16(s[0]) << 8) | uint16(s[1])
}

// Return true if name added successfully, false otherwise
func (d *domainSet) Add(str string) bool {
	// To reduce memory, we don't use full qualified name

	name, ok := stringToDomain(str)
	if !ok {
		var err error
		name, err = idna.ToASCII(str)
		// idna.ToASCII("") return no error
		if err != nil || len(name) == 0 {
			return false
		}
	}

	// To speed up name lookup, we utilized two-way hash
	// The first one is the first two ASCII characters of the domain name
	// The second one is the real domain set
	// Which works somewhat like ordinary English dictionary lookup
	s := (*d)[domainToIndex(name)]
	if s == nil {
		// MT-Unsafe: Initialize real domain set on demand
		s = make(StringSet)
		(*d)[domainToIndex(name)] = s
	}
	s.Add(name)
	return true
}

// for loop will exit in advance if f() return error
func (d *domainSet) ForEachDomain(f func(name string) error) error {
	for _, s := range *d {
		for name := range s {
			if err := f(name); err != nil {
				return err
			}
		}
	}
	return nil
}

// Assume `child' is lower cased and without trailing dot
func (d *domainSet) Match(child string) bool {
	if d.Len() == 0 {
		return false
	}
	if len(child) == 0 {
		panic(fmt.Sprintf("Why child is an empty string?!"))
	}

	for {
		s := (*d)[domainToIndex(child)]
		// Fast lookup for a full match
		if s != nil && s.Contains(child) {
			return true
		}

		// Fallback to iterate the whole set
		for parent := range s {
			if plugin.Name(parent).Matches(child) {
				return true
			}
		}

		i := strings.Index(child, ".")
		if i <= 0 {
			break
		}
		child = child[i+1:]
	}

	return false
}
