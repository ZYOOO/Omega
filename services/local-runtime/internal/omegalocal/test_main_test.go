package omegalocal

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	if os.Getenv("OMEGA_FEISHU_AUTO_DELIVERY_DISABLED") == "" {
		_ = os.Setenv("OMEGA_FEISHU_AUTO_DELIVERY_DISABLED", "1")
	}
	os.Exit(m.Run())
}

func enableFeishuAutoDeliveryForTest(t *testing.T) {
	t.Helper()
	t.Setenv("OMEGA_FEISHU_AUTO_DELIVERY_DISABLED", "")
}
