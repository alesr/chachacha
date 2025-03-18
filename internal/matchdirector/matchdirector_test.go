package matchdirector

import (
	"context"
	"testing"
	"time"

	"github.com/alesr/chachacha/internal/sessionrepo"
	pubevts "github.com/alesr/chachacha/pkg/events"
	"github.com/alesr/chachacha/pkg/logutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	t.Parallel()

	givenLogger := logutils.NewNoop()
	givenRepo := &repoMock{}
	givenMatchInterval := 100 * time.Millisecond

	md, err := New(givenLogger, givenRepo, givenMatchInterval)

	require.NoError(t, err)
	require.NotNil(t, md)

	assert.Equal(t, givenLogger.WithGroup("match_director"), md.logger)
	assert.Equal(t, givenRepo, md.repo)
	assert.NotZero(t, md.matchTicker)
	assert.NotNil(t, md.stopChan)
}

func TestMatchDirector_StartStop(t *testing.T) {
	t.Parallel()

	logger := logutils.NewNoop()

	repo := repoMock{
		getHostsFunc: func(ctx context.Context) ([]pubevts.HostRegistratioEvent, error) {
			return []pubevts.HostRegistratioEvent{}, nil
		},
		getPlayersFunc: func(ctx context.Context) ([]pubevts.MatchRequestEvent, error) {
			return []pubevts.MatchRequestEvent{}, nil
		},
	}

	md, err := New(logger, &repo, 50*time.Millisecond)
	require.NoError(t, err)

	md.Start()

	// wait for at least one matching cycle
	time.Sleep(time.Second * 3)

	// should have called the repo methods at least once
	assert.True(t, repo.wasGetHostsCalled())
	assert.True(t, repo.wasGetPlayersCalled())

	repo.resetCalls()

	md.Stop()

	// no more calls should happen after stopping
	assert.False(t, repo.wasGetHostsCalled())
	assert.False(t, repo.wasGetPlayersCalled())
}

func TestMatchDirector_matchPlayers(t *testing.T) {
	t.Parallel()

	dmMode := pubevts.GameMode("deathmatch")
	ctfMode := pubevts.GameMode("capture-flag")

	host1 := pubevts.HostRegistratioEvent{
		HostID:         "host1",
		Mode:           dmMode,
		AvailableSlots: 2,
	}

	host2 := pubevts.HostRegistratioEvent{
		HostID:         "host2",
		Mode:           ctfMode,
		AvailableSlots: 3,
	}

	// player wanting specific host and mode
	player1 := pubevts.MatchRequestEvent{
		PlayerID: "player1",
		HostID:   stringToPtr("host1"),
		Mode:     &dmMode,
	}

	// player wanting any host with specific mode
	player2 := pubevts.MatchRequestEvent{
		PlayerID: "player2",
		Mode:     &dmMode,
	}

	// player wanting specific host with no mode preference
	player3 := pubevts.MatchRequestEvent{
		PlayerID: "player3",
		HostID:   stringToPtr("host2"),
	}

	// player with no preferences
	player4 := pubevts.MatchRequestEvent{
		PlayerID: "player4",
	}

	// player requesting ctf mode
	player5 := pubevts.MatchRequestEvent{
		PlayerID: "player5",
		Mode:     &ctfMode,
	}

	// player with invalid host specification
	player6 := pubevts.MatchRequestEvent{
		PlayerID: "player6",
		HostID:   stringToPtr("non-existent-host"),
	}

	// player with incompatible mode for host
	player7 := pubevts.MatchRequestEvent{
		PlayerID: "player7",
		HostID:   stringToPtr("host1"),
		Mode:     &ctfMode, // host1 offers deathmatch
	}

	testCases := []struct {
		name                  string
		givenHosts            []pubevts.HostRegistratioEvent
		givenPlayers          []pubevts.MatchRequestEvent
		expectError           bool
		expectSessionsCreated int
		expectRemovedPlayers  []string
		expectStoredSessions  []*sessionrepo.Session
	}{
		{
			name:                  "no hosts available",
			givenHosts:            []pubevts.HostRegistratioEvent{},
			givenPlayers:          []pubevts.MatchRequestEvent{player1, player2},
			expectError:           false, // no error, just no matches
			expectSessionsCreated: 0,
			expectRemovedPlayers:  []string{},
			expectStoredSessions:  []*sessionrepo.Session{},
		},
		{
			name:                  "no players available",
			givenHosts:            []pubevts.HostRegistratioEvent{host1, host2},
			givenPlayers:          []pubevts.MatchRequestEvent{},
			expectError:           false, // no error, just no matches
			expectSessionsCreated: 0,
			expectRemovedPlayers:  []string{},
			expectStoredSessions:  []*sessionrepo.Session{},
		},
		{
			name:                  "match player with specific host and mode",
			givenHosts:            []pubevts.HostRegistratioEvent{host1},
			givenPlayers:          []pubevts.MatchRequestEvent{player1},
			expectError:           false,
			expectSessionsCreated: 1,
			expectRemovedPlayers:  []string{"player1"},
			expectStoredSessions: []*sessionrepo.Session{
				{
					// ID is random and will be checked separately
					HostID:         "host1",
					Mode:           dmMode,
					Players:        []string{"player1"},
					AvailableSlots: 1, // started with 2, used 1
				},
			},
		},
		{
			name:                  "match player with any host but specific mode",
			givenHosts:            []pubevts.HostRegistratioEvent{host1, host2},
			givenPlayers:          []pubevts.MatchRequestEvent{player2, player5},
			expectError:           false,
			expectSessionsCreated: 2,
			expectRemovedPlayers:  []string{"player2", "player5"},
			expectStoredSessions: []*sessionrepo.Session{
				{
					HostID:         "host1",
					Mode:           dmMode,
					Players:        []string{"player2"},
					AvailableSlots: 1, // started with 2, used 1
				},
				{
					HostID:         "host2",
					Mode:           ctfMode,
					Players:        []string{"player5"},
					AvailableSlots: 2, // started with 3, used 1
				},
			},
		},
		{
			name:                  "match multiple players to single host",
			givenHosts:            []pubevts.HostRegistratioEvent{host1},
			givenPlayers:          []pubevts.MatchRequestEvent{player1, player2},
			expectError:           false,
			expectSessionsCreated: 1,
			expectRemovedPlayers:  []string{"player1", "player2"},
			expectStoredSessions: []*sessionrepo.Session{
				{
					HostID:         "host1",
					Mode:           dmMode,
					Players:        []string{"player1", "player2"},
					AvailableSlots: 0, // started with 2, used 2
				},
			},
		},
		{
			name:                  "player with invalid host not matched",
			givenHosts:            []pubevts.HostRegistratioEvent{host1, host2},
			givenPlayers:          []pubevts.MatchRequestEvent{player6},
			expectError:           false,
			expectSessionsCreated: 0, // No matches should be found
			expectRemovedPlayers:  []string{},
			expectStoredSessions:  []*sessionrepo.Session{},
		},
		{
			name:                  "player with incompatible mode not matched",
			givenHosts:            []pubevts.HostRegistratioEvent{host1},
			givenPlayers:          []pubevts.MatchRequestEvent{player7},
			expectError:           false,
			expectSessionsCreated: 0, // No matches should be found
			expectRemovedPlayers:  []string{},
			expectStoredSessions:  []*sessionrepo.Session{},
		},
		{
			name:                  "complex scenario with multiple hosts and players",
			givenHosts:            []pubevts.HostRegistratioEvent{host1, host2},
			givenPlayers:          []pubevts.MatchRequestEvent{player1, player2, player3, player4, player5, player6, player7},
			expectError:           false,
			expectSessionsCreated: 2,
			// player1, player2 to host1; player3, player4, player5 to host2; player6, player7 not matched
			expectRemovedPlayers: []string{"player1", "player2", "player3", "player4", "player5"},
			expectStoredSessions: []*sessionrepo.Session{
				{
					HostID:         "host1",
					Mode:           dmMode,
					Players:        []string{"player1", "player2"},
					AvailableSlots: 0, // started with 2, used 2
				},
				{
					HostID:         "host2",
					Mode:           ctfMode,
					Players:        []string{"player3", "player5", "player4"},
					AvailableSlots: 0, // started with 3, used 3
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			logger := logutils.NewNoop()

			// track removed players
			removedPlayers := make([]string, 0)
			storedSessions := make([]*sessionrepo.Session, 0)

			repo := &repoMock{
				getHostsFunc: func(ctx context.Context) ([]pubevts.HostRegistratioEvent, error) {
					return tc.givenHosts, nil
				},
				getPlayersFunc: func(ctx context.Context) ([]pubevts.MatchRequestEvent, error) {
					return tc.givenPlayers, nil
				},
				storeGameSessionFunc: func(ctx context.Context, session *sessionrepo.Session) error {
					// copying the session to avoid modifications after the fact
					sessionCopy := &sessionrepo.Session{
						ID:             session.ID,
						HostID:         session.HostID,
						Mode:           session.Mode,
						CreatedAt:      session.CreatedAt,
						Players:        make([]string, len(session.Players)),
						AvailableSlots: session.AvailableSlots,
					}
					copy(sessionCopy.Players, session.Players)
					storedSessions = append(storedSessions, sessionCopy)
					return nil
				},
				updateHostAvailableSlotsFunc: func(ctx context.Context, hostIP string, slots uint16) error {
					return nil
				},
				removePlayerFunc: func(ctx context.Context, playerID string) error {
					removedPlayers = append(removedPlayers, playerID)
					return nil
				},
			}

			md, err := New(logger, repo, 1*time.Second)
			require.NoError(t, err)

			err = md.matchPlayers()
			if tc.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, tc.expectSessionsCreated, len(storedSessions))

			assert.ElementsMatch(t, tc.expectRemovedPlayers, removedPlayers)

			if len(tc.expectStoredSessions) > 0 {
				require.Len(t, storedSessions, len(tc.expectStoredSessions))

				// Track which expected sessions have been matched
				matchedSessions := make([]bool, len(tc.expectStoredSessions))

				// For each stored session, find a matching expected session
				for _, stored := range storedSessions {
					var found bool
					for i, expected := range tc.expectStoredSessions {
						if !matchedSessions[i] && // not already matched
							stored.HostID == expected.HostID &&
							stored.Mode == expected.Mode &&
							stored.AvailableSlots == expected.AvailableSlots &&
							assert.ElementsMatch(t, expected.Players, stored.Players) {

							// mark this expected session as matched
							matchedSessions[i] = true
							found = true
							break
						}
					}
					assert.True(t, found, "Could not find matching expected session for stored session: %+v", stored)
				}

				for i, matched := range matchedSessions {
					assert.True(t, matched, "Expected session not matched: %+v", tc.expectStoredSessions[i])
				}
			}
		})
	}
}

func stringToPtr(s string) *string {
	return &s
}
