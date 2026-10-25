package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/evcc-io/evcc/core"
	"github.com/evcc-io/evcc/core/settings"
	"github.com/evcc-io/evcc/util"
	"github.com/stretchr/testify/assert"
)

func TestChargingRestrictionsAPI(t *testing.T) {
	lp := core.NewLoadpoint(util.NewLogger("test"), settings.NewMemorySettings())
	handler := chargingRestrictionsHandler(lp)
	for _, tc := range []struct {
		body string
		code int
	}{
		{`{"enabled":true,"timezone":"Europe/Stockholm","rules":[{"months":[1,2,3,11,12],"weekdays":[1,2,3,4,5],"start":"07:00","end":"20:00"}]}`, 200},
		{`{"enabled":true,"timezone":"Europe/Stockholm","rules":[]}`, 400},
		{`{"enabled":true,"timezone":"invalid","rules":[]}`, 400},
		{`{"enabled":true,"timezone":"Europe/Stockholm","rules":[{"start":"7:00","end":"20:00"}]}`, 400},
		{`{"unexpected":true}`, 400},
	} {
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tc.body)))
		assert.Equal(t, tc.code, res.Code, res.Body.String())
	}
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/", nil))
	assert.Equal(t, 200, res.Code)
	assert.Contains(t, res.Body.String(), `"start":"07:00"`, "invalid updates preserve the saved rule")
}
