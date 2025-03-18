package sessionrepo

import (
	"context"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
)

var _ redisClient = &redisMock{}

type redisMock struct {
	mu             sync.Mutex
	data           map[string]string
	sets           map[string]map[string]struct{}
	setCalled      bool
	sAddCalled     bool
	getCalled      bool
	sRemCalled     bool
	sMembersCalled bool
	delCalled      bool
	setFunc        func(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.StatusCmd
	sAddFunc       func(ctx context.Context, key string, members ...interface{}) *redis.IntCmd
	getFunc        func(ctx context.Context, key string) *redis.StringCmd
	sRemFunc       func(ctx context.Context, key string, members ...interface{}) *redis.IntCmd
	sMembersFunc   func(ctx context.Context, key string) *redis.StringSliceCmd
	delFunc        func(ctx context.Context, keys ...string) *redis.IntCmd
}

func newRedisMock() *redisMock {
	return &redisMock{
		data: make(map[string]string),
		sets: make(map[string]map[string]struct{}),
	}
}

func (m *redisMock) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.StatusCmd {
	m.mu.Lock()
	m.setCalled = true
	m.mu.Unlock()

	if m.setFunc != nil {
		return m.setFunc(ctx, key, value, expiration)
	}

	strValue, ok := value.(string)
	if !ok {
		if byteValue, ok := value.([]byte); ok {
			strValue = string(byteValue)
		}
	}

	m.mu.Lock()
	m.data[key] = strValue
	m.mu.Unlock()
	return redis.NewStatusCmd(ctx)
}

func (m *redisMock) SAdd(ctx context.Context, key string, members ...interface{}) *redis.IntCmd {
	m.mu.Lock()
	m.sAddCalled = true
	m.mu.Unlock()

	if m.sAddFunc != nil {
		return m.sAddFunc(ctx, key, members...)
	}

	m.mu.Lock()
	if _, exists := m.sets[key]; !exists {
		m.sets[key] = make(map[string]struct{})
	}

	added := int64(0)
	for _, member := range members {
		strMember := member.(string)
		if _, exists := m.sets[key][strMember]; !exists {
			m.sets[key][strMember] = struct{}{}
			added++
		}
	}
	m.mu.Unlock()

	cmd := redis.NewIntCmd(ctx)
	cmd.SetVal(added)
	return cmd
}

func (m *redisMock) Get(ctx context.Context, key string) *redis.StringCmd {
	m.mu.Lock()
	m.getCalled = true
	m.mu.Unlock()

	if m.getFunc != nil {
		return m.getFunc(ctx, key)
	}

	cmd := redis.NewStringCmd(ctx)
	m.mu.Lock()
	val, exists := m.data[key]
	m.mu.Unlock()

	if !exists {
		cmd.SetErr(redis.Nil)
		return cmd
	}

	cmd.SetVal(val)
	return cmd
}

func (m *redisMock) SRem(ctx context.Context, key string, members ...interface{}) *redis.IntCmd {
	m.mu.Lock()
	m.sRemCalled = true
	m.mu.Unlock()

	if m.sRemFunc != nil {
		return m.sRemFunc(ctx, key, members...)
	}

	m.mu.Lock()
	removed := int64(0)
	if set, exists := m.sets[key]; exists {
		for _, member := range members {
			strMember := member.(string)
			if _, exists := set[strMember]; exists {
				delete(set, strMember)
				removed++
			}
		}
	}
	m.mu.Unlock()

	cmd := redis.NewIntCmd(ctx)
	cmd.SetVal(removed)
	return cmd
}

func (m *redisMock) SMembers(ctx context.Context, key string) *redis.StringSliceCmd {
	m.mu.Lock()
	m.sMembersCalled = true
	m.mu.Unlock()

	if m.sMembersFunc != nil {
		return m.sMembersFunc(ctx, key)
	}

	cmd := redis.NewStringSliceCmd(ctx)
	m.mu.Lock()
	if set, exists := m.sets[key]; exists {
		members := make([]string, 0, len(set))
		for member := range set {
			members = append(members, member)
		}
		cmd.SetVal(members)
	} else {
		cmd.SetVal([]string{})
	}
	m.mu.Unlock()
	return cmd
}

func (m *redisMock) Del(ctx context.Context, keys ...string) *redis.IntCmd {
	m.mu.Lock()
	m.delCalled = true
	m.mu.Unlock()

	if m.delFunc != nil {
		return m.delFunc(ctx, keys...)
	}

	m.mu.Lock()
	deleted := int64(0)
	for _, key := range keys {
		if _, exists := m.data[key]; exists {
			delete(m.data, key)
			deleted++
		}
	}
	m.mu.Unlock()

	cmd := redis.NewIntCmd(ctx)
	cmd.SetVal(deleted)
	return cmd
}

func (m *redisMock) wasSetCalled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.setCalled
}

func (m *redisMock) wasSAddCalled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sAddCalled
}

func (m *redisMock) wasGetCalled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.getCalled
}

func (m *redisMock) wasSRemCalled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sRemCalled
}

func (m *redisMock) wasSMembersCalled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sMembersCalled
}

func (m *redisMock) wasDelCalled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.delCalled
}
