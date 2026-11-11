// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

using System.Net;
using Xunit;

namespace Peaceful.Canton.Localnet.Testing.Tests;

public class TransientHttpStatusTests
{
    [Theory]
    [InlineData(HttpStatusCode.ServiceUnavailable, "", true)]
    [InlineData(HttpStatusCode.GatewayTimeout, "", true)]
    [InlineData(HttpStatusCode.Conflict, """{"code":"REQUEST_ALREADY_IN_FLIGHT"}""", true)]
    [InlineData(HttpStatusCode.Conflict, """{"code":"PARTY_ALREADY_EXISTS"}""", false)]
    [InlineData(HttpStatusCode.ServiceUnavailable, """{"code":"REQUEST_ALREADY_IN_FLIGHT"}""", true)]
    [InlineData(HttpStatusCode.BadGateway, "", false)]
    [InlineData(HttpStatusCode.InternalServerError, "", false)]
    [InlineData(HttpStatusCode.TooManyRequests, "", false)]
    [InlineData(HttpStatusCode.Unauthorized, "", false)]
    [InlineData(HttpStatusCode.BadRequest, """{"code":"REQUEST_ALREADY_IN_FLIGHT"}""", false)]
    public void IsTransient_matches_the_status_set_the_go_fixture_retries(HttpStatusCode status, string body, bool expected)
    {
        Assert.Equal(expected, TransientHttpStatus.IsTransient(status, body));
    }
}
