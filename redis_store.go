package apibudget

import (
	"context"
	"crypto/tls"
	"fmt"
	"math/big"
	"time"

	"github.com/redis/go-redis/v9"
)

// redisConfig はRedisストアの設定。
type redisConfig struct {
	password  string
	db        int
	enableTLS bool
}

// RedisOption はRedisストアの設定オプション。
type RedisOption func(*redisConfig)

// WithRedisPassword はRedisパスワードを設定する。
func WithRedisPassword(password string) RedisOption {
	return func(c *redisConfig) {
		c.password = password
	}
}

// WithRedisDB はRedis DB番号を設定する。
func WithRedisDB(db int) RedisOption {
	return func(c *redisConfig) {
		c.db = db
	}
}

// WithRedisTLS はRedis TLS接続を有効にする。
func WithRedisTLS(enable bool) RedisOption {
	return func(c *redisConfig) {
		c.enableTLS = enable
	}
}

// redisStore はRedisベースのStore実装。原子性はLuaスクリプトで保証する。
type redisStore struct {
	client *redis.Client
}

// Luaスクリプト定義

// luaIncrementWindow: INCRBY + PEXPIRE（新規キー作成時のみTTL設定）
var luaIncrementWindow = redis.NewScript(`
local current = redis.call('INCRBY', KEYS[1], ARGV[1])
if current == tonumber(ARGV[1]) then
  redis.call('PEXPIRE', KEYS[1], ARGV[2])
end
return current
`)

// luaDecrementWindow: DECRBY（最小0）
var luaDecrementWindow = redis.NewScript(`
local current = redis.call('GET', KEYS[1])
if current == false then
  return 0
end
local val = tonumber(current) - tonumber(ARGV[1])
if val < 0 then
  val = 0
end
redis.call('SET', KEYS[1], tostring(val))
local ttl = redis.call('PTTL', KEYS[1])
if ttl > 0 then
  redis.call('PEXPIRE', KEYS[1], ttl)
end
return val
`)

// luaDeductCredit: 原子的なcheck-and-deduct
var luaDeductCredit = redis.NewScript(`
local current = redis.call('GET', KEYS[1])
if current == false then
  return redis.error_reply('pool not found')
end
local cur_num = current
local cur_rat = cur_num
local amt = ARGV[1]
-- Use string-based rational arithmetic via Lua
-- Parse as rational numbers: "a/b" or plain number
local function parse_rat(s)
  local slash = string.find(s, '/')
  if slash then
    return tonumber(string.sub(s, 1, slash-1)), tonumber(string.sub(s, slash+1))
  else
    local n = tonumber(s)
    if n == nil then return 0, 1 end
    -- Convert float to rational approximation
    -- For integers, just return n/1
    return n, 1
  end
end
local cur_n, cur_d = parse_rat(current)
local amt_n, amt_d = parse_rat(amt)
-- remaining = cur - amt = (cur_n*amt_d - amt_n*cur_d) / (cur_d*amt_d)
local rem_n = cur_n * amt_d - amt_n * cur_d
local rem_d = cur_d * amt_d
if rem_n < 0 then
  return redis.error_reply('insufficient credits')
end
-- Simplify: find GCD
local function gcd(a, b)
  a = math.abs(a)
  b = math.abs(b)
  while b ~= 0 do
    a, b = b, a % b
  end
  return a
end
local g = gcd(rem_n, rem_d)
rem_n = rem_n / g
rem_d = rem_d / g
local result
if rem_d == 1 then
  result = tostring(rem_n)
else
  result = tostring(rem_n) .. '/' .. tostring(rem_d)
end
redis.call('SET', KEYS[1], result)
return result
`)

// luaAddCredit: 原子的なadd
var luaAddCredit = redis.NewScript(`
local current = redis.call('GET', KEYS[1])
if current == false then
  return redis.error_reply('pool not found')
end
local function parse_rat(s)
  local slash = string.find(s, '/')
  if slash then
    return tonumber(string.sub(s, 1, slash-1)), tonumber(string.sub(s, slash+1))
  else
    local n = tonumber(s)
    if n == nil then return 0, 1 end
    return n, 1
  end
end
local cur_n, cur_d = parse_rat(current)
local amt_n, amt_d = parse_rat(ARGV[1])
-- result = cur + amt = (cur_n*amt_d + amt_n*cur_d) / (cur_d*amt_d)
local res_n = cur_n * amt_d + amt_n * cur_d
local res_d = cur_d * amt_d
local function gcd(a, b)
  a = math.abs(a)
  b = math.abs(b)
  while b ~= 0 do
    a, b = b, a % b
  end
  return a
end
local g = gcd(res_n, res_d)
res_n = res_n / g
res_d = res_d / g
local result
if res_d == 1 then
  result = tostring(res_n)
else
  result = tostring(res_n) .. '/' .. tostring(res_d)
end
redis.call('SET', KEYS[1], result)
return result
`)

// NewRedisStore はRedisベースのストアを生成する。
// 接続に失敗した場合はエラーを返す。
func NewRedisStore(addr string, opts ...RedisOption) (Store, error) {
	cfg := &redisConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	options := &redis.Options{
		Addr:     addr,
		Password: cfg.password,
		DB:       cfg.db,
	}
	if cfg.enableTLS {
		options.TLSConfig = &tls.Config{
			MinVersion: tls.VersionTLS12,
		}
	}

	client := redis.NewClient(options)

	// 接続確認
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("apibudget: redis connection failed: %w", err)
	}

	return &redisStore{client: client}, nil
}

// IncrementWindow はキーのカウンタをdelta分増加し、現在値を返す。
// 新規キー作成時のみTTLを設定する。
func (s *redisStore) IncrementWindow(ctx context.Context, key string, delta int64, window time.Duration) (int64, error) {
	ttlMs := window.Milliseconds()
	result, err := luaIncrementWindow.Run(ctx, s.client, []string{key}, delta, ttlMs).Int64()
	if err != nil {
		return 0, fmt.Errorf("apibudget: redis IncrementWindow failed: %w", err)
	}
	return result, nil
}

// DecrementWindow はキーのカウンタをdelta分減少させる（最小0）。
func (s *redisStore) DecrementWindow(ctx context.Context, key string, delta int64) error {
	_, err := luaDecrementWindow.Run(ctx, s.client, []string{key}, delta).Int64()
	if err != nil {
		return fmt.Errorf("apibudget: redis DecrementWindow failed: %w", err)
	}
	return nil
}

// GetWindowCount は現在の窓内のカウント値を返す。
func (s *redisStore) GetWindowCount(ctx context.Context, key string) (int64, error) {
	val, err := s.client.Get(ctx, key).Int64()
	if err == redis.Nil {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("apibudget: redis GetWindowCount failed: %w", err)
	}
	return val, nil
}

// GetCredit はクレジットプールの残高を取得する。
func (s *redisStore) GetCredit(ctx context.Context, poolKey string) (Credit, error) {
	val, err := s.client.Get(ctx, poolKey).Result()
	if err == redis.Nil {
		return Credit{}, ErrPoolNotFound
	}
	if err != nil {
		return Credit{}, fmt.Errorf("apibudget: redis GetCredit failed: %w", err)
	}
	return parseRatString(val)
}

// SetCredit はクレジットプールの残高を設定する。
func (s *redisStore) SetCredit(ctx context.Context, poolKey string, value Credit) error {
	err := s.client.Set(ctx, poolKey, value.ensureVal().RatString(), 0).Err()
	if err != nil {
		return fmt.Errorf("apibudget: redis SetCredit failed: %w", err)
	}
	return nil
}

// DeductCredit はクレジットを原子的に減算する。残高不足ならエラーを返し残高を変更しない。
func (s *redisStore) DeductCredit(ctx context.Context, poolKey string, amount Credit) (Credit, error) {
	result, err := luaDeductCredit.Run(ctx, s.client, []string{poolKey}, amount.ensureVal().RatString()).Result()
	if err != nil {
		if err.Error() == "pool not found" {
			return Credit{}, ErrPoolNotFound
		}
		if err.Error() == "insufficient credits" {
			// 残高を取得して返す
			current, getErr := s.GetCredit(ctx, poolKey)
			if getErr != nil {
				return Credit{}, ErrInsufficientCredits
			}
			return current, ErrInsufficientCredits
		}
		return Credit{}, fmt.Errorf("apibudget: redis DeductCredit failed: %w", err)
	}
	return parseRatString(fmt.Sprintf("%v", result))
}

// AddCredit はクレジットを加算する。
func (s *redisStore) AddCredit(ctx context.Context, poolKey string, amount Credit) (Credit, error) {
	result, err := luaAddCredit.Run(ctx, s.client, []string{poolKey}, amount.ensureVal().RatString()).Result()
	if err != nil {
		if err.Error() == "pool not found" {
			return Credit{}, ErrPoolNotFound
		}
		return Credit{}, fmt.Errorf("apibudget: redis AddCredit failed: %w", err)
	}
	return parseRatString(fmt.Sprintf("%v", result))
}

// Close はRedisクライアントを閉じる。
func (s *redisStore) Close() error {
	return s.client.Close()
}

// parseRatString はRedisから取得した文字列をCreditに変換する。
func parseRatString(s string) (Credit, error) {
	r := new(big.Rat)
	if _, ok := r.SetString(s); !ok {
		return Credit{}, fmt.Errorf("apibudget: invalid credit value from redis: %q", s)
	}
	return Credit{val: r}, nil
}
