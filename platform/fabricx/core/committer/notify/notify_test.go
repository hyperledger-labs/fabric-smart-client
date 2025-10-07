package notify

import (
	"fmt"
	"testing"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/vault/queryservice"
	"github.com/hyperledger/fabric-x-committer/api/protonotify"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/durationpb"
)

func TestNotificationService(t *testing.T) {

	table := []struct {
		name   string
		cfg    map[string]any
		checks func(t *testing.T, client *grpc.ClientConn, err error)
	}{
		{
			name: "connect",
			cfg: map[string]any{
				"queryService.queryTimeout": 10 * time.Second,
				"queryService.Endpoints": []any{
					map[string]any{
						"address":           "localhost:5411",
						"connectionTimeout": 0 * time.Second,
					},
				},
			},
			checks: func(t *testing.T, client *grpc.ClientConn, err error) {
				t.Helper()
				require.NotNil(t, client)
				require.NoError(t, err)

				nf := protonotify.NewNotifierClient(client)

				notifyStream, err := nf.OpenNotificationStream(t.Context())
				require.NoError(t, err)

				txIDs := make([]string, 1)
				err = notifyStream.Send(&protonotify.NotificationRequest{
					TxStatusRequest: &protonotify.TxStatusRequest{
						TxIds: txIDs,
					},
					Timeout: durationpb.New(3 * time.Minute),
				})
				require.NoError(t, err)

				var actual []*protonotify.TxStatusEvent
				require.EventuallyWithT(t, func(ct *assert.CollectT) {
					res, err := notifyStream.Recv()
					require.NoError(t, err)
					require.NotNil(t, res)
					require.Nil(t, res.TimeoutTxIds)
					actual = append(actual, res.TxStatusEvents...)
					//test.RequireProtoElementsMatch(ct, expected, actual)
				}, 15*time.Second, 50*time.Millisecond)
			},
		},
	}

	for _, tc := range table {
		t.Run(fmt.Sprintf("grpcClient %v", tc.name), func(t *testing.T) {
			t.Parallel()
			cs := newConfigService(tc.cfg)
			c, err := queryservice.NewConfig(cs)
			client, err := queryservice.GrpcClient(c)
			require.NoError(t, err)
			tc.checks(t, client, err)
		})
	}

}

type configService struct {
	V *viper.Viper
}

func newConfigService(c map[string]any) *configService {
	v := viper.New()
	for k, val := range c {
		v.Set(k, val)
	}
	return &configService{V: v}
}

func (c configService) UnmarshalKey(key string, rawVal interface{}) error {
	return c.V.UnmarshalKey(key, rawVal)
}
