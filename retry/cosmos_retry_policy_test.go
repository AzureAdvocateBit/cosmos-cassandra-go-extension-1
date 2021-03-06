package retry

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gocql/gocql"
	"github.com/stretchr/testify/assert"
)

const rateLimitedErrMsg = `Request rate is large: ActivityID=c268afb6-7367-4ff8-b06b-b7e2d1269f55, RetryAfterMs=42, Additional details='Response status code does not indicate success: TooManyRequests (429); Substatus: 3200; ActivityId: c268afb6-7367-4ff8-b06b-b7e2d1269f55; Reason: ({
	"Errors": [
	  "Request rate is large. More Request Units may be needed, so no changes were made. Please retry this request later. Learn more: http://aka.ms/cosmosdb-error-429"
	]
  });`

const rateLimitedErrMsgWithoutRetryAfterMs = `Request rate is large: ActivityID=c268afb6-7367-4ff8-b06b-b7e2d1269f55, Additional details='Response status code does not indicate success: TooManyRequests (429); Substatus: 3200; ActivityId: c268afb6-7367-4ff8-b06b-b7e2d1269f55; Reason: ({
	"Errors": [
	  "Request rate is large. More Request Units may be needed, so no changes were made. Please retry this request later. Learn more: http://aka.ms/cosmosdb-error-429"
	]
  });`

func TestRetryAllowed(t *testing.T) {
	type testCase struct {
		name   string
		policy *CosmosRetryPolicy
		result bool
	}

	testCases := []testCase{
		{"will attempt to retry if max retry count is infinite", NewCosmosRetryPolicy(-1), true},
		{"will attempt to retry if max retry count is finite", NewCosmosRetryPolicy(5), true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(te *testing.T) {
			actual := tc.policy.Attempt(MockRetryableQuery{})
			assert.True(te, actual, "query will not be retried")
		})
	}

}

func TestRetryDuration(t *testing.T) {
	type testCase struct {
		name           string
		policy         *CosmosRetryPolicy
		errMsg         string
		expectedResult time.Duration
	}
	p := NewCosmosRetryPolicy(5)
	testCases := []testCase{
		{"retry duration for rate limited error", p, rateLimitedErrMsg, time.Duration(42) * time.Millisecond},
		{"retry duration for rate limited error when RetryAfterMs is not available", p, rateLimitedErrMsgWithoutRetryAfterMs, time.Duration(p.FixedBackOffTimeMs) * time.Millisecond},
		{"retry duration for errors other than rate limiting", p, "error: today is not your day!", time.Duration(-1)},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(te *testing.T) {
			expectedRetryAfterMs := time.Duration(tc.expectedResult)
			actualRetryAfterMs := tc.policy.getRetryAfterMs(tc.errMsg)
			assert.Equal(te, expectedRetryAfterMs, actualRetryAfterMs)
		})
	}
}

func TestRetryDurationForRateLimitedErrorInfiniteRetryWhenRetryMsUnavailable(t *testing.T) {
	p := NewCosmosRetryPolicy(-1) // infinite retry
	p.numAttempts = 2             // assuming the query has been retried twice already

	actualRetryAfterMs := p.getRetryAfterMs(rateLimitedErrMsgWithoutRetryAfterMs)
	// since numAttempts is 2, the retry duration will be more than 2s
	threshold := time.Duration(2) * time.Second
	if actualRetryAfterMs < threshold {
		t.Errorf("expected retry duration - %v. actual - %v", threshold, actualRetryAfterMs)
	}
}

func TestGetRetryType(t *testing.T) {
	type testCase struct {
		name string
		//errorType         gocql.RequestError
		errorType         error
		expectedRetryType gocql.RetryType
	}

	testCases := []testCase{
		{"retry type for RequestErrReadTimeout", &gocql.RequestErrReadTimeout{}, gocql.Retry},
		{"retry type for RequestErrUnavailable", &gocql.RequestErrUnavailable{}, gocql.Retry},
		{"retry type for RequestErrWriteTimeout", &gocql.RequestErrWriteTimeout{}, gocql.Retry},
		{"retry type for rate limited error", errors.New(rateLimitedErrMsg), gocql.Retry},
		{"retry type for rate limited error when RetryAfterMs is unavailable", errors.New(rateLimitedErrMsgWithoutRetryAfterMs), gocql.Retry},
		{"retry type for error other than rate limiting", errors.New("error: today is not your day"), gocql.Rethrow},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(te *testing.T) {
			p := NewCosmosRetryPolicy(2)
			actualRetryType := p.GetRetryType(tc.errorType)
			expectedRetryType := tc.expectedRetryType
			assert.Equal(te, expectedRetryType, actualRetryType)
		})
	}
}

type MockRetryableQuery struct {
}

func (mrq MockRetryableQuery) Attempts() int {
	return 0
}
func (mrq MockRetryableQuery) SetConsistency(c gocql.Consistency) {
}
func (mrq MockRetryableQuery) GetConsistency() gocql.Consistency {
	return gocql.Any
}
func (mrq MockRetryableQuery) Context() context.Context {
	return context.Background()
}
