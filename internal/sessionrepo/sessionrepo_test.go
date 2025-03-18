package sessionrepo

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/alesr/chachacha/pkg/game"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRedisRepo(t *testing.T) {
	t.Parallel()

	redisMock := newRedisMock()
	repo, err := NewRedisRepo(redisMock)

	require.NoError(t, err)
	require.NotNil(t, repo)
	assert.Equal(t, redisMock, repo.client)
}

func TestRedis_StoreHost(t *testing.T) {
	t.Parallel()

	givenHost := game.HostRegistratioMessage{
		HostID:         "host-123",
		Mode:           game.GameMode("deathmatch"),
		AvailableSlots: 4,
	}

	testCases := []struct {
		name           string
		setFunc        func(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.StatusCmd
		sAddFunc       func(ctx context.Context, key string, members ...interface{}) *redis.IntCmd
		expectErr      bool
		expectSetCall  bool
		expectSAddCall bool
	}{
		{
			name: "successful store",
			setFunc: func(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.StatusCmd {
				assert.Equal(t, hostKeyPrefix+givenHost.HostID, key)
				assert.Equal(t, hostTTL, expiration)

				// Verify the JSON data
				valueBytes, ok := value.([]byte)
				assert.True(t, ok)

				var host game.HostRegistratioMessage
				err := json.Unmarshal(valueBytes, &host)
				assert.NoError(t, err)
				assert.Equal(t, givenHost, host)

				cmd := redis.NewStatusCmd(ctx)
				cmd.SetVal("OK")
				return cmd
			},
			sAddFunc: func(ctx context.Context, key string, members ...interface{}) *redis.IntCmd {
				assert.Equal(t, hostsSetKey, key)
				assert.Equal(t, 1, len(members))
				assert.Equal(t, hostKeyPrefix+givenHost.HostID, members[0])

				cmd := redis.NewIntCmd(ctx)
				cmd.SetVal(1)
				return cmd
			},
			expectErr:      false,
			expectSetCall:  true,
			expectSAddCall: true,
		},
		{
			name: "set fails",
			setFunc: func(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.StatusCmd {
				cmd := redis.NewStatusCmd(ctx)
				cmd.SetErr(assert.AnError)
				return cmd
			},
			sAddFunc: func(ctx context.Context, key string, members ...interface{}) *redis.IntCmd {
				cmd := redis.NewIntCmd(ctx)
				cmd.SetVal(1)
				return cmd
			},
			expectErr:      true,
			expectSetCall:  true,
			expectSAddCall: false,
		},
		{
			name: "sadd fails",
			setFunc: func(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.StatusCmd {
				cmd := redis.NewStatusCmd(ctx)
				cmd.SetVal("OK")
				return cmd
			},
			sAddFunc: func(ctx context.Context, key string, members ...interface{}) *redis.IntCmd {
				cmd := redis.NewIntCmd(ctx)
				cmd.SetErr(assert.AnError)
				return cmd
			},
			expectErr:      true,
			expectSetCall:  true,
			expectSAddCall: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			redisMock := newRedisMock()
			redisMock.setFunc = tc.setFunc
			redisMock.sAddFunc = tc.sAddFunc

			repo, err := NewRedisRepo(redisMock)
			require.NoError(t, err)

			err = repo.StoreHost(context.Background(), givenHost)
			if tc.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, tc.expectSetCall, redisMock.wasSetCalled())
			assert.Equal(t, tc.expectSAddCall, redisMock.wasSAddCalled())
		})
	}
}

func TestRedis_UpdateHostAvailableSlots(t *testing.T) {
	t.Parallel()

	hostID := "host-123"
	originalSlots := int8(4)
	updatedSlots := int8(2)

	givenHost := game.HostRegistratioMessage{
		HostID:         hostID,
		Mode:           game.GameMode("deathmatch"),
		AvailableSlots: originalSlots,
	}

	testCases := []struct {
		name          string
		getFunc       func(ctx context.Context, key string) *redis.StringCmd
		setFunc       func(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.StatusCmd
		expectErr     bool
		expectGetCall bool
		expectSetCall bool
	}{
		{
			name: "successful update",
			getFunc: func(ctx context.Context, key string) *redis.StringCmd {
				assert.Equal(t, hostKeyPrefix+hostID, key)

				data, err := json.Marshal(givenHost)
				assert.NoError(t, err)

				cmd := redis.NewStringCmd(ctx)
				cmd.SetVal(string(data))
				return cmd
			},
			setFunc: func(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.StatusCmd {
				assert.Equal(t, hostKeyPrefix+hostID, key)
				assert.Equal(t, hostTTL, expiration)

				// Verify the JSON data has updated slots
				valueBytes, ok := value.([]byte)
				assert.True(t, ok)

				var host game.HostRegistratioMessage
				err := json.Unmarshal(valueBytes, &host)
				assert.NoError(t, err)
				assert.Equal(t, updatedSlots, host.AvailableSlots)

				cmd := redis.NewStatusCmd(ctx)
				cmd.SetVal("OK")
				return cmd
			},
			expectErr:     false,
			expectGetCall: true,
			expectSetCall: true,
		},
		{
			name: "get fails",
			getFunc: func(ctx context.Context, key string) *redis.StringCmd {
				cmd := redis.NewStringCmd(ctx)
				cmd.SetErr(assert.AnError)
				return cmd
			},
			setFunc: func(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.StatusCmd {
				cmd := redis.NewStatusCmd(ctx)
				cmd.SetVal("OK")
				return cmd
			},
			expectErr:     true,
			expectGetCall: true,
			expectSetCall: false,
		},
		{
			name: "set fails",
			getFunc: func(ctx context.Context, key string) *redis.StringCmd {
				data, err := json.Marshal(givenHost)
				assert.NoError(t, err)

				cmd := redis.NewStringCmd(ctx)
				cmd.SetVal(string(data))
				return cmd
			},
			setFunc: func(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.StatusCmd {
				cmd := redis.NewStatusCmd(ctx)
				cmd.SetErr(assert.AnError)
				return cmd
			},
			expectErr:     true,
			expectGetCall: true,
			expectSetCall: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			redisMock := newRedisMock()
			redisMock.getFunc = tc.getFunc
			redisMock.setFunc = tc.setFunc

			repo, err := NewRedisRepo(redisMock)
			require.NoError(t, err)

			err = repo.UpdateHostAvailableSlots(context.Background(), hostID, updatedSlots)
			if tc.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, tc.expectGetCall, redisMock.wasGetCalled())
			assert.Equal(t, tc.expectSetCall, redisMock.wasSetCalled())
		})
	}
}

func TestRedis_StorePlayer(t *testing.T) {
	t.Parallel()

	gameMode := game.GameMode("deathmatch")
	hostID := "host-123"
	givenPlayer := game.MatchRequestMessage{
		PlayerID: "player-456",
		HostID:   &hostID,
		Mode:     &gameMode,
	}

	testCases := []struct {
		name           string
		setFunc        func(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.StatusCmd
		sAddFunc       func(ctx context.Context, key string, members ...interface{}) *redis.IntCmd
		expectErr      bool
		expectSetCall  bool
		expectSAddCall bool
	}{
		{
			name: "successful store",
			setFunc: func(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.StatusCmd {
				assert.Equal(t, playerKeyPrefix+givenPlayer.PlayerID, key)
				assert.Equal(t, playerTTL, expiration)

				// Verify the JSON data
				valueBytes, ok := value.([]byte)
				assert.True(t, ok)

				var player game.MatchRequestMessage
				err := json.Unmarshal(valueBytes, &player)
				assert.NoError(t, err)
				assert.Equal(t, givenPlayer, player)

				cmd := redis.NewStatusCmd(ctx)
				cmd.SetVal("OK")
				return cmd
			},
			sAddFunc: func(ctx context.Context, key string, members ...interface{}) *redis.IntCmd {
				assert.Equal(t, playersSetKey, key)
				assert.Equal(t, 1, len(members))
				assert.Equal(t, playerKeyPrefix+givenPlayer.PlayerID, members[0])

				cmd := redis.NewIntCmd(ctx)
				cmd.SetVal(1)
				return cmd
			},
			expectErr:      false,
			expectSetCall:  true,
			expectSAddCall: true,
		},
		{
			name: "set fails",
			setFunc: func(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.StatusCmd {
				cmd := redis.NewStatusCmd(ctx)
				cmd.SetErr(assert.AnError)
				return cmd
			},
			sAddFunc: func(ctx context.Context, key string, members ...interface{}) *redis.IntCmd {
				cmd := redis.NewIntCmd(ctx)
				cmd.SetVal(1)
				return cmd
			},
			expectErr:      true,
			expectSetCall:  true,
			expectSAddCall: false,
		},
		{
			name: "sadd fails",
			setFunc: func(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.StatusCmd {
				cmd := redis.NewStatusCmd(ctx)
				cmd.SetVal("OK")
				return cmd
			},
			sAddFunc: func(ctx context.Context, key string, members ...interface{}) *redis.IntCmd {
				cmd := redis.NewIntCmd(ctx)
				cmd.SetErr(assert.AnError)
				return cmd
			},
			expectErr:      true,
			expectSetCall:  true,
			expectSAddCall: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			redisMock := newRedisMock()
			redisMock.setFunc = tc.setFunc
			redisMock.sAddFunc = tc.sAddFunc

			repo, err := NewRedisRepo(redisMock)
			require.NoError(t, err)

			err = repo.StorePlayer(context.Background(), givenPlayer)
			if tc.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, tc.expectSetCall, redisMock.wasSetCalled())
			assert.Equal(t, tc.expectSAddCall, redisMock.wasSAddCalled())
		})
	}
}

func TestRedis_RemovePlayer(t *testing.T) {
	t.Parallel()

	playerID := "player-456"
	playerKey := playerKeyPrefix + playerID

	testCases := []struct {
		name           string
		sRemFunc       func(ctx context.Context, key string, members ...interface{}) *redis.IntCmd
		delFunc        func(ctx context.Context, keys ...string) *redis.IntCmd
		expectErr      bool
		expectSRemCall bool
		expectDelCall  bool
	}{
		{
			name: "successful removal",
			sRemFunc: func(ctx context.Context, key string, members ...interface{}) *redis.IntCmd {
				assert.Equal(t, playersSetKey, key)
				assert.Equal(t, 1, len(members))
				assert.Equal(t, playerKey, members[0])

				cmd := redis.NewIntCmd(ctx)
				cmd.SetVal(1)
				return cmd
			},
			delFunc: func(ctx context.Context, keys ...string) *redis.IntCmd {
				assert.Equal(t, 1, len(keys))
				assert.Equal(t, playerKey, keys[0])

				cmd := redis.NewIntCmd(ctx)
				cmd.SetVal(1)
				return cmd
			},
			expectErr:      false,
			expectSRemCall: true,
			expectDelCall:  true,
		},
		{
			name: "srem fails",
			sRemFunc: func(ctx context.Context, key string, members ...interface{}) *redis.IntCmd {
				cmd := redis.NewIntCmd(ctx)
				cmd.SetErr(assert.AnError)
				return cmd
			},
			delFunc: func(ctx context.Context, keys ...string) *redis.IntCmd {
				cmd := redis.NewIntCmd(ctx)
				cmd.SetVal(1)
				return cmd
			},
			expectErr:      true,
			expectSRemCall: true,
			expectDelCall:  false,
		},
		{
			name: "del fails",
			sRemFunc: func(ctx context.Context, key string, members ...interface{}) *redis.IntCmd {
				cmd := redis.NewIntCmd(ctx)
				cmd.SetVal(1)
				return cmd
			},
			delFunc: func(ctx context.Context, keys ...string) *redis.IntCmd {
				cmd := redis.NewIntCmd(ctx)
				cmd.SetErr(assert.AnError)
				return cmd
			},
			expectErr:      true,
			expectSRemCall: true,
			expectDelCall:  true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			redisMock := newRedisMock()
			redisMock.sRemFunc = tc.sRemFunc
			redisMock.delFunc = tc.delFunc

			repo, err := NewRedisRepo(redisMock)
			require.NoError(t, err)

			err = repo.RemovePlayer(context.Background(), playerID)
			if tc.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, tc.expectSRemCall, redisMock.wasSRemCalled())
			assert.Equal(t, tc.expectDelCall, redisMock.wasDelCalled())
		})
	}
}

func TestRedis_StoreGameSession(t *testing.T) {
	t.Parallel()

	givenSession := &Session{
		ID:             "session-789",
		HostID:         "host-123",
		Mode:           game.GameMode("deathmatch"),
		CreatedAt:      time.Now(),
		Players:        []string{"player-456", "player-457"},
		AvailableSlots: 2,
	}

	testCases := []struct {
		name           string
		setFunc        func(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.StatusCmd
		sAddFunc       func(ctx context.Context, key string, members ...interface{}) *redis.IntCmd
		expectErr      bool
		expectSetCall  bool
		expectSAddCall bool
	}{
		{
			name: "successful store",
			setFunc: func(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.StatusCmd {
				assert.Equal(t, sessionKeyPrefix+givenSession.ID, key)
				assert.Equal(t, sessionTTL, expiration)

				// Verify the JSON data
				valueBytes, ok := value.([]byte)
				assert.True(t, ok)

				var session Session
				err := json.Unmarshal(valueBytes, &session)
				assert.NoError(t, err)
				assert.Equal(t, givenSession.ID, session.ID)
				assert.Equal(t, givenSession.HostID, session.HostID)
				assert.Equal(t, givenSession.Mode, session.Mode)
				assert.Equal(t, givenSession.AvailableSlots, session.AvailableSlots)
				assert.Equal(t, givenSession.Players, session.Players)

				cmd := redis.NewStatusCmd(ctx)
				cmd.SetVal("OK")
				return cmd
			},
			sAddFunc: func(ctx context.Context, key string, members ...interface{}) *redis.IntCmd {
				assert.Equal(t, activeSessionsKey, key)
				assert.Equal(t, 1, len(members))
				assert.Equal(t, sessionKeyPrefix+givenSession.ID, members[0])

				cmd := redis.NewIntCmd(ctx)
				cmd.SetVal(1)
				return cmd
			},
			expectErr:      false,
			expectSetCall:  true,
			expectSAddCall: true,
		},
		{
			name: "set fails",
			setFunc: func(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.StatusCmd {
				cmd := redis.NewStatusCmd(ctx)
				cmd.SetErr(assert.AnError)
				return cmd
			},
			sAddFunc: func(ctx context.Context, key string, members ...interface{}) *redis.IntCmd {
				cmd := redis.NewIntCmd(ctx)
				cmd.SetVal(1)
				return cmd
			},
			expectErr:      true,
			expectSetCall:  true,
			expectSAddCall: false,
		},
		{
			name: "sadd fails",
			setFunc: func(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.StatusCmd {
				cmd := redis.NewStatusCmd(ctx)
				cmd.SetVal("OK")
				return cmd
			},
			sAddFunc: func(ctx context.Context, key string, members ...interface{}) *redis.IntCmd {
				cmd := redis.NewIntCmd(ctx)
				cmd.SetErr(assert.AnError)
				return cmd
			},
			expectErr:      true,
			expectSetCall:  true,
			expectSAddCall: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			redisMock := newRedisMock()
			redisMock.setFunc = tc.setFunc
			redisMock.sAddFunc = tc.sAddFunc

			repo, err := NewRedisRepo(redisMock)
			require.NoError(t, err)

			err = repo.StoreGameSession(context.Background(), givenSession)
			if tc.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, tc.expectSetCall, redisMock.wasSetCalled())
			assert.Equal(t, tc.expectSAddCall, redisMock.wasSAddCalled())
		})
	}
}

func TestRedis_GetGameSession(t *testing.T) {
	t.Parallel()

	sessionID := "session-789"
	sessionKey := sessionKeyPrefix + sessionID

	givenSession := &Session{
		ID:             sessionID,
		HostID:         "host-123",
		Mode:           game.GameMode("deathmatch"),
		CreatedAt:      time.Now(),
		Players:        []string{"player-456", "player-457"},
		AvailableSlots: 2,
	}

	sessionJSON, err := json.Marshal(givenSession)
	require.NoError(t, err)

	testCases := []struct {
		name          string
		getFunc       func(ctx context.Context, key string) *redis.StringCmd
		expectErr     bool
		expectGetCall bool
		expectSession *Session
	}{
		{
			name: "successful get",
			getFunc: func(ctx context.Context, key string) *redis.StringCmd {
				assert.Equal(t, sessionKey, key)

				cmd := redis.NewStringCmd(ctx)
				cmd.SetVal(string(sessionJSON))
				return cmd
			},
			expectErr:     false,
			expectGetCall: true,
			expectSession: givenSession,
		},
		{
			name: "session not found",
			getFunc: func(ctx context.Context, key string) *redis.StringCmd {
				cmd := redis.NewStringCmd(ctx)
				cmd.SetErr(redis.Nil)
				return cmd
			},
			expectErr:     true,
			expectGetCall: true,
			expectSession: nil,
		},
		{
			name: "redis error",
			getFunc: func(ctx context.Context, key string) *redis.StringCmd {
				cmd := redis.NewStringCmd(ctx)
				cmd.SetErr(assert.AnError)
				return cmd
			},
			expectErr:     true,
			expectGetCall: true,
			expectSession: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			redisMock := newRedisMock()
			redisMock.getFunc = tc.getFunc

			repo, err := NewRedisRepo(redisMock)
			require.NoError(t, err)

			session, err := repo.GetGameSession(context.Background(), sessionID)
			if tc.expectErr {
				assert.Error(t, err)
				assert.Nil(t, session)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expectSession.ID, session.ID)
				assert.Equal(t, tc.expectSession.HostID, session.HostID)
				assert.Equal(t, tc.expectSession.Mode, session.Mode)
				assert.Equal(t, tc.expectSession.AvailableSlots, session.AvailableSlots)
				assert.Equal(t, tc.expectSession.Players, session.Players)
			}

			assert.Equal(t, tc.expectGetCall, redisMock.wasGetCalled())
		})
	}
}

func TestRedis_GetActiveGameSessions(t *testing.T) {
	t.Parallel()

	session1 := &Session{
		ID:             "session-789",
		HostID:         "host-123",
		Mode:           game.GameMode("deathmatch"),
		CreatedAt:      time.Now(),
		Players:        []string{"player-456", "player-457"},
		AvailableSlots: 2,
	}

	session2 := &Session{
		ID:             "session-790",
		HostID:         "host-124",
		Mode:           game.GameMode("capture-flag"),
		CreatedAt:      time.Now(),
		Players:        []string{"player-458", "player-459"},
		AvailableSlots: 4,
	}

	sessionKey1 := sessionKeyPrefix + session1.ID
	sessionKey2 := sessionKeyPrefix + session2.ID

	sessionJSON1, err := json.Marshal(session1)
	require.NoError(t, err)

	sessionJSON2, err := json.Marshal(session2)
	require.NoError(t, err)

	testCases := []struct {
		name               string
		sMembersFunc       func(ctx context.Context, key string) *redis.StringSliceCmd
		getFunc            func(ctx context.Context, key string) *redis.StringCmd
		expectErr          bool
		expectSMembersCall bool
		expectGetCall      bool
		expectSessions     []*Session
	}{
		{
			name: "successful get multiple sessions",
			sMembersFunc: func(ctx context.Context, key string) *redis.StringSliceCmd {
				assert.Equal(t, activeSessionsKey, key)

				cmd := redis.NewStringSliceCmd(ctx)
				cmd.SetVal([]string{sessionKey1, sessionKey2})
				return cmd
			},
			getFunc: func(ctx context.Context, key string) *redis.StringCmd {
				cmd := redis.NewStringCmd(ctx)
				if key == sessionKey1 {
					cmd.SetVal(string(sessionJSON1))
				} else if key == sessionKey2 {
					cmd.SetVal(string(sessionJSON2))
				}
				return cmd
			},
			expectErr:          false,
			expectSMembersCall: true,
			expectGetCall:      true,
			expectSessions:     []*Session{session1, session2},
		},
		{
			name: "smembers fails",
			sMembersFunc: func(ctx context.Context, key string) *redis.StringSliceCmd {
				cmd := redis.NewStringSliceCmd(ctx)
				cmd.SetErr(assert.AnError)
				return cmd
			},
			getFunc: func(ctx context.Context, key string) *redis.StringCmd {
				cmd := redis.NewStringCmd(ctx)
				return cmd
			},
			expectErr:          true,
			expectSMembersCall: true,
			expectGetCall:      false,
			expectSessions:     nil,
		},
		{
			name: "no active sessions",
			sMembersFunc: func(ctx context.Context, key string) *redis.StringSliceCmd {
				cmd := redis.NewStringSliceCmd(ctx)
				cmd.SetVal([]string{})
				return cmd
			},
			getFunc: func(ctx context.Context, key string) *redis.StringCmd {
				cmd := redis.NewStringCmd(ctx)
				return cmd
			},
			expectErr:          false,
			expectSMembersCall: true,
			expectGetCall:      false,
			expectSessions:     []*Session{},
		},
		{
			name: "session expired but still in set",
			sMembersFunc: func(ctx context.Context, key string) *redis.StringSliceCmd {
				cmd := redis.NewStringSliceCmd(ctx)
				cmd.SetVal([]string{sessionKey1})
				return cmd
			},
			getFunc: func(ctx context.Context, key string) *redis.StringCmd {
				cmd := redis.NewStringCmd(ctx)
				cmd.SetErr(redis.Nil)
				return cmd
			},
			expectErr:          false,
			expectSMembersCall: true,
			expectGetCall:      true,
			expectSessions:     []*Session{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			redisMock := newRedisMock()
			redisMock.sMembersFunc = tc.sMembersFunc
			redisMock.getFunc = tc.getFunc

			repo, err := NewRedisRepo(redisMock)
			require.NoError(t, err)

			sessions, err := repo.GetActiveGameSessions(context.Background())
			if tc.expectErr {
				assert.Error(t, err)
				assert.Nil(t, sessions)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, len(tc.expectSessions), len(sessions))

				if len(tc.expectSessions) > 0 {
					for i, expected := range tc.expectSessions {
						assert.Equal(t, expected.ID, sessions[i].ID)
						assert.Equal(t, expected.HostID, sessions[i].HostID)
						assert.Equal(t, expected.Mode, sessions[i].Mode)
						assert.Equal(t, expected.AvailableSlots, sessions[i].AvailableSlots)
						assert.Equal(t, expected.Players, sessions[i].Players)
					}
				}
			}

			assert.Equal(t, tc.expectSMembersCall, redisMock.wasSMembersCalled())
			if tc.expectGetCall {
				assert.True(t, redisMock.wasGetCalled())
			}
		})
	}
}

func TestRedis_GetHosts(t *testing.T) {
	t.Parallel()

	host1 := game.HostRegistratioMessage{
		HostID:         "host-123",
		Mode:           game.GameMode("deathmatch"),
		AvailableSlots: 4,
	}

	host2 := game.HostRegistratioMessage{
		HostID:         "host-124",
		Mode:           game.GameMode("capture-flag"),
		AvailableSlots: 6,
	}

	hostKey1 := hostKeyPrefix + host1.HostID
	hostKey2 := hostKeyPrefix + host2.HostID

	hostJSON1, err := json.Marshal(host1)
	require.NoError(t, err)

	hostJSON2, err := json.Marshal(host2)
	require.NoError(t, err)

	testCases := []struct {
		name               string
		sMembersFunc       func(ctx context.Context, key string) *redis.StringSliceCmd
		getFunc            func(ctx context.Context, key string) *redis.StringCmd
		expectErr          bool
		expectSMembersCall bool
		expectGetCall      bool
		expectHosts        []game.HostRegistratioMessage
	}{
		{
			name: "successful get multiple hosts",
			sMembersFunc: func(ctx context.Context, key string) *redis.StringSliceCmd {
				assert.Equal(t, hostsSetKey, key)

				cmd := redis.NewStringSliceCmd(ctx)
				cmd.SetVal([]string{hostKey1, hostKey2})
				return cmd
			},
			getFunc: func(ctx context.Context, key string) *redis.StringCmd {
				cmd := redis.NewStringCmd(ctx)
				if key == hostKey1 {
					cmd.SetVal(string(hostJSON1))
				} else if key == hostKey2 {
					cmd.SetVal(string(hostJSON2))
				}
				return cmd
			},
			expectErr:          false,
			expectSMembersCall: true,
			expectGetCall:      true,
			expectHosts:        []game.HostRegistratioMessage{host1, host2},
		},
		{
			name: "smembers fails",
			sMembersFunc: func(ctx context.Context, key string) *redis.StringSliceCmd {
				cmd := redis.NewStringSliceCmd(ctx)
				cmd.SetErr(assert.AnError)
				return cmd
			},
			getFunc: func(ctx context.Context, key string) *redis.StringCmd {
				cmd := redis.NewStringCmd(ctx)
				return cmd
			},
			expectErr:          true,
			expectSMembersCall: true,
			expectGetCall:      false,
			expectHosts:        nil,
		},
		{
			name: "no hosts",
			sMembersFunc: func(ctx context.Context, key string) *redis.StringSliceCmd {
				cmd := redis.NewStringSliceCmd(ctx)
				cmd.SetVal([]string{})
				return cmd
			},
			getFunc: func(ctx context.Context, key string) *redis.StringCmd {
				cmd := redis.NewStringCmd(ctx)
				return cmd
			},
			expectErr:          false,
			expectSMembersCall: true,
			expectGetCall:      false,
			expectHosts:        []game.HostRegistratioMessage{},
		},
		{
			name: "host expired but still in set",
			sMembersFunc: func(ctx context.Context, key string) *redis.StringSliceCmd {
				cmd := redis.NewStringSliceCmd(ctx)
				cmd.SetVal([]string{hostKey1})
				return cmd
			},
			getFunc: func(ctx context.Context, key string) *redis.StringCmd {
				cmd := redis.NewStringCmd(ctx)
				cmd.SetErr(redis.Nil)
				return cmd
			},
			expectErr:          false,
			expectSMembersCall: true,
			expectGetCall:      true,
			expectHosts:        []game.HostRegistratioMessage{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			redisMock := newRedisMock()
			redisMock.sMembersFunc = tc.sMembersFunc
			redisMock.getFunc = tc.getFunc

			repo, err := NewRedisRepo(redisMock)
			require.NoError(t, err)

			hosts, err := repo.GetHosts(context.Background())
			if tc.expectErr {
				assert.Error(t, err)
				assert.Nil(t, hosts)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, len(tc.expectHosts), len(hosts))

				if len(tc.expectHosts) > 0 {
					for i, expected := range tc.expectHosts {
						assert.Equal(t, expected.HostID, hosts[i].HostID)
						assert.Equal(t, expected.Mode, hosts[i].Mode)
						assert.Equal(t, expected.AvailableSlots, hosts[i].AvailableSlots)
					}
				}
			}

			assert.Equal(t, tc.expectSMembersCall, redisMock.wasSMembersCalled())
			if tc.expectGetCall {
				assert.True(t, redisMock.wasGetCalled())
			}
		})
	}
}

func TestRedis_GetPlayers(t *testing.T) {
	t.Parallel()

	gameMode1 := game.GameMode("deathmatch")
	gameMode2 := game.GameMode("capture-flag")
	hostID1 := "host-123"
	hostID2 := "host-124"

	player1 := game.MatchRequestMessage{
		PlayerID: "player-456",
		HostID:   &hostID1,
		Mode:     &gameMode1,
	}

	player2 := game.MatchRequestMessage{
		PlayerID: "player-457",
		HostID:   &hostID2,
		Mode:     &gameMode2,
	}

	playerKey1 := playerKeyPrefix + player1.PlayerID
	playerKey2 := playerKeyPrefix + player2.PlayerID

	playerJSON1, err := json.Marshal(player1)
	require.NoError(t, err)

	playerJSON2, err := json.Marshal(player2)
	require.NoError(t, err)

	testCases := []struct {
		name               string
		sMembersFunc       func(ctx context.Context, key string) *redis.StringSliceCmd
		getFunc            func(ctx context.Context, key string) *redis.StringCmd
		expectErr          bool
		expectSMembersCall bool
		expectGetCall      bool
		expectPlayers      []game.MatchRequestMessage
	}{
		{
			name: "successful get multiple players",
			sMembersFunc: func(ctx context.Context, key string) *redis.StringSliceCmd {
				assert.Equal(t, playersSetKey, key)

				cmd := redis.NewStringSliceCmd(ctx)
				cmd.SetVal([]string{playerKey1, playerKey2})
				return cmd
			},
			getFunc: func(ctx context.Context, key string) *redis.StringCmd {
				cmd := redis.NewStringCmd(ctx)
				if key == playerKey1 {
					cmd.SetVal(string(playerJSON1))
				} else if key == playerKey2 {
					cmd.SetVal(string(playerJSON2))
				}
				return cmd
			},
			expectErr:          false,
			expectSMembersCall: true,
			expectGetCall:      true,
			expectPlayers:      []game.MatchRequestMessage{player1, player2},
		},
		{
			name: "smembers fails",
			sMembersFunc: func(ctx context.Context, key string) *redis.StringSliceCmd {
				cmd := redis.NewStringSliceCmd(ctx)
				cmd.SetErr(assert.AnError)
				return cmd
			},
			getFunc: func(ctx context.Context, key string) *redis.StringCmd {
				cmd := redis.NewStringCmd(ctx)
				return cmd
			},
			expectErr:          true,
			expectSMembersCall: true,
			expectGetCall:      false,
			expectPlayers:      nil,
		},
		{
			name: "no players",
			sMembersFunc: func(ctx context.Context, key string) *redis.StringSliceCmd {
				cmd := redis.NewStringSliceCmd(ctx)
				cmd.SetVal([]string{})
				return cmd
			},
			getFunc: func(ctx context.Context, key string) *redis.StringCmd {
				cmd := redis.NewStringCmd(ctx)
				return cmd
			},
			expectErr:          false,
			expectSMembersCall: true,
			expectGetCall:      false,
			expectPlayers:      []game.MatchRequestMessage{},
		},
		{
			name: "player expired but still in set",
			sMembersFunc: func(ctx context.Context, key string) *redis.StringSliceCmd {
				cmd := redis.NewStringSliceCmd(ctx)
				cmd.SetVal([]string{playerKey1})
				return cmd
			},
			getFunc: func(ctx context.Context, key string) *redis.StringCmd {
				cmd := redis.NewStringCmd(ctx)
				cmd.SetErr(redis.Nil)
				return cmd
			},
			expectErr:          false,
			expectSMembersCall: true,
			expectGetCall:      true,
			expectPlayers:      []game.MatchRequestMessage{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			redisMock := newRedisMock()
			redisMock.sMembersFunc = tc.sMembersFunc
			redisMock.getFunc = tc.getFunc

			repo, err := NewRedisRepo(redisMock)
			require.NoError(t, err)

			players, err := repo.GetPlayers(context.Background())
			if tc.expectErr {
				assert.Error(t, err)
				assert.Nil(t, players)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, len(tc.expectPlayers), len(players))

				if len(tc.expectPlayers) > 0 {
					for i, expected := range tc.expectPlayers {
						assert.Equal(t, expected.PlayerID, players[i].PlayerID)
						assert.Equal(t, *expected.HostID, *players[i].HostID)
						assert.Equal(t, *expected.Mode, *players[i].Mode)
					}
				}
			}

			assert.Equal(t, tc.expectSMembersCall, redisMock.wasSMembersCalled())
			if tc.expectGetCall {
				assert.True(t, redisMock.wasGetCalled())
			}
		})
	}
}
