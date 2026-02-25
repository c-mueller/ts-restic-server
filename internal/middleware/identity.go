package middleware

import (
	"container/list"
	"context"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
)

type identityKey struct{}
type whoIsResultKey struct{}

// GetIdentity returns the resolved identities (IP, FQDN, short hostname) from the request context.
func GetIdentity(ctx context.Context) []string {
	if v, ok := ctx.Value(identityKey{}).([]string); ok {
		return v
	}
	return nil
}

// GetWhoIsResult returns the WhoIs result from the request context, or nil if not available.
func GetWhoIsResult(ctx context.Context) *WhoIsResult {
	if v, ok := ctx.Value(whoIsResultKey{}).(*WhoIsResult); ok {
		return v
	}
	return nil
}

func setIdentity(c echo.Context, ids []string) {
	ctx := context.WithValue(c.Request().Context(), identityKey{}, ids)
	c.SetRequest(c.Request().WithContext(ctx))
}

func setWhoIsResult(c echo.Context, result *WhoIsResult) {
	ctx := context.WithValue(c.Request().Context(), whoIsResultKey{}, result)
	c.SetRequest(c.Request().WithContext(ctx))
}

// rdnsCacheItem stores a single rDNS cache entry with its key for FIFO eviction.
type rdnsCacheItem struct {
	key       string
	hostnames []string
	expiry    time.Time
}

// rdnsCache caches rDNS lookup results with a configurable TTL and bounded size.
// When maxSize is reached, the oldest entry (FIFO) is evicted.
type rdnsCache struct {
	mu      sync.RWMutex
	entries map[string]*list.Element
	order   *list.List
	ttl     time.Duration
	maxSize int
}

func newRDNSCache(ttl time.Duration, maxSize int) *rdnsCache {
	return &rdnsCache{
		entries: make(map[string]*list.Element),
		order:   list.New(),
		ttl:     ttl,
		maxSize: maxSize,
	}
}

func (c *rdnsCache) Get(ip string) ([]string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	elem, ok := c.entries[ip]
	if !ok {
		return nil, false
	}
	item := elem.Value.(*rdnsCacheItem)
	if time.Now().After(item.expiry) {
		return nil, false
	}
	return item.hostnames, true
}

func (c *rdnsCache) Set(ip string, hostnames []string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Update existing entry: remove old element, will re-add at back.
	if elem, ok := c.entries[ip]; ok {
		c.order.Remove(elem)
		delete(c.entries, ip)
	}

	// Evict oldest entry if at capacity.
	if len(c.entries) >= c.maxSize {
		oldest := c.order.Front()
		if oldest != nil {
			item := oldest.Value.(*rdnsCacheItem)
			c.order.Remove(oldest)
			delete(c.entries, item.key)
		}
	}

	item := &rdnsCacheItem{
		key:       ip,
		hostnames: hostnames,
		expiry:    time.Now().Add(c.ttl),
	}
	elem := c.order.PushBack(item)
	c.entries[ip] = elem
}

// RDNSIdentity returns middleware that resolves client IPs to hostnames via rDNS.
//
// Parameters:
//   - dnsServer: DNS server for rDNS ("" = system default, "100.100.100.100:53" for Tailscale)
//   - cacheTTL: how long to cache rDNS results
//   - includeShortHostname: if true, add the first label of the FQDN (Tailscale mode)
//   - maxCacheSize: maximum number of entries in the identity cache
//   - logger: for debug/warn logging
func RDNSIdentity(dnsServer string, cacheTTL time.Duration, includeShortHostname bool, maxCacheSize int, logger *zap.Logger) echo.MiddlewareFunc {
	var resolver *net.Resolver
	if dnsServer == "" {
		resolver = &net.Resolver{PreferGo: true}
	} else {
		resolver = &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
				d := &net.Dialer{Timeout: 3 * time.Second}
				return d.DialContext(ctx, "udp", dnsServer)
			},
		}
	}

	cache := newRDNSCache(cacheTTL, maxCacheSize)

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			ip := c.RealIP()

			// Check cache
			if hostnames, ok := cache.Get(ip); ok {
				ids := buildIdentifiers(ip, hostnames, includeShortHostname)
				setIdentity(c, ids)
				return next(c)
			}

			// rDNS lookup with timeout
			ctx, cancel := context.WithTimeout(c.Request().Context(), 3*time.Second)
			names, err := resolver.LookupAddr(ctx, ip)
			cancel()

			var hostnames []string
			if err != nil {
				logger.Debug("rdns lookup failed", zap.String("ip", ip), zap.Error(err))
				// Negative cache — avoid repeated lookups
				cache.Set(ip, nil)
				setIdentity(c, []string{ip})
				return next(c)
			}

			for _, name := range names {
				h := strings.TrimSuffix(name, ".")
				if h != "" {
					hostnames = append(hostnames, h)
				}
			}

			cache.Set(ip, hostnames)
			ids := buildIdentifiers(ip, hostnames, includeShortHostname)
			logger.Debug("rdns resolved", zap.String("ip", ip), zap.Strings("identities", ids))
			setIdentity(c, ids)
			return next(c)
		}
	}
}

// buildIdentifiers constructs the identity list: [ip, fqdn(s)..., shortHostname(s)...].
func buildIdentifiers(ip string, hostnames []string, includeShort bool) []string {
	ids := []string{ip}
	ids = append(ids, hostnames...)
	if includeShort {
		for _, h := range hostnames {
			if short, _, ok := strings.Cut(h, "."); ok && short != "" {
				ids = append(ids, short)
			}
		}
	}
	return ids
}

// WhoIsResult contains resolved identity info from a Tailscale WhoIs lookup.
type WhoIsResult struct {
	FQDN      string   // e.g. "server.tailnet.ts.net"
	ShortName string   // e.g. "server"
	Tags      []string // e.g. ["tag:server", "tag:backup"]
	LoginName string   // e.g. "alice@example.com"
}

// WhoIsFunc resolves a remote address (ip:port) to identity info.
type WhoIsFunc func(ctx context.Context, remoteAddr string) (*WhoIsResult, error)

// whoIsCacheItem stores a single WhoIs cache entry with its key for FIFO eviction.
type whoIsCacheItem struct {
	key    string
	ids    []string
	result *WhoIsResult // nil for negative cache entries
	expiry time.Time
}

// whoIsCache caches WhoIs lookup results (identities + structured result) with a configurable TTL
// and bounded size. When maxSize is reached, the oldest entry (FIFO) is evicted.
type whoIsCache struct {
	mu      sync.RWMutex
	entries map[string]*list.Element
	order   *list.List
	ttl     time.Duration
	maxSize int
}

func newWhoIsCache(ttl time.Duration, maxSize int) *whoIsCache {
	return &whoIsCache{
		entries: make(map[string]*list.Element),
		order:   list.New(),
		ttl:     ttl,
		maxSize: maxSize,
	}
}

func (c *whoIsCache) Get(ip string) ([]string, *WhoIsResult, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	elem, ok := c.entries[ip]
	if !ok {
		return nil, nil, false
	}
	item := elem.Value.(*whoIsCacheItem)
	if time.Now().After(item.expiry) {
		return nil, nil, false
	}
	return item.ids, item.result, true
}

func (c *whoIsCache) Set(ip string, ids []string, result *WhoIsResult) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Update existing entry: remove old element, will re-add at back.
	if elem, ok := c.entries[ip]; ok {
		c.order.Remove(elem)
		delete(c.entries, ip)
	}

	// Evict oldest entry if at capacity.
	if len(c.entries) >= c.maxSize {
		oldest := c.order.Front()
		if oldest != nil {
			item := oldest.Value.(*whoIsCacheItem)
			c.order.Remove(oldest)
			delete(c.entries, item.key)
		}
	}

	item := &whoIsCacheItem{
		key:    ip,
		ids:    ids,
		result: result,
		expiry: time.Now().Add(c.ttl),
	}
	elem := c.order.PushBack(item)
	c.entries[ip] = elem
}

// WhoIsIdentity returns middleware that resolves client identities via Tailscale WhoIs.
// This provides richer identity info than rDNS: tags, user login, hostname.
func WhoIsIdentity(whoIs WhoIsFunc, cacheTTL time.Duration, maxCacheSize int, logger *zap.Logger) echo.MiddlewareFunc {
	cache := newWhoIsCache(cacheTTL, maxCacheSize)

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			ip := c.RealIP()

			// Check cache
			if ids, result, ok := cache.Get(ip); ok {
				setIdentity(c, ids)
				if result != nil {
					setWhoIsResult(c, result)
				}
				return next(c)
			}

			// WhoIs lookup with timeout
			ctx, cancel := context.WithTimeout(c.Request().Context(), 5*time.Second)
			result, err := whoIs(ctx, c.Request().RemoteAddr)
			cancel()

			if err != nil {
				logger.Debug("whois lookup failed", zap.String("ip", ip), zap.Error(err))
				// Negative cache — avoid repeated lookups
				cache.Set(ip, []string{ip}, nil)
				setIdentity(c, []string{ip})
				return next(c)
			}

			ids := buildWhoIsIdentifiers(ip, result)
			cache.Set(ip, ids, result)
			logger.Debug("whois resolved", zap.String("ip", ip), zap.Strings("identities", ids))
			setIdentity(c, ids)
			setWhoIsResult(c, result)
			return next(c)
		}
	}
}

// buildWhoIsIdentifiers constructs the identity list from a WhoIs result:
// [ip, fqdn, shortName, loginName, tags...].
func buildWhoIsIdentifiers(ip string, result *WhoIsResult) []string {
	ids := []string{ip}
	if result.FQDN != "" {
		ids = append(ids, result.FQDN)
	}
	if result.ShortName != "" {
		ids = append(ids, result.ShortName)
	}
	if result.LoginName != "" {
		ids = append(ids, result.LoginName)
	}
	ids = append(ids, result.Tags...)
	return ids
}

// PlainIdentity returns middleware that sets identity to just the client IP.
// Used when no ACL is configured.
func PlainIdentity() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			setIdentity(c, []string{c.RealIP()})
			return next(c)
		}
	}
}
