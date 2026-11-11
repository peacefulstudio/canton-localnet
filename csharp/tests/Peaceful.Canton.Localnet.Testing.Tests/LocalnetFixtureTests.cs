// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

using System.Collections.Concurrent;
using System.Net;
using System.Net.Sockets;
using System.Text;
using Microsoft.Extensions.Logging;
using Xunit;

namespace Peaceful.Canton.Localnet.Testing.Tests;

public class LocalnetFixtureTests
{
    private const string AdminClientCategory = "Peaceful.Canton.Localnet.Testing.JsonLedgerAdminClient";
    private const string DiscoveryLogMessageSuffix = "v2/state/connected-synchronizers";

    [Fact]
    public async Task UploadDarAsync_logs_synchronizer_discovery_through_the_fixture_logger_factory()
    {
        using var ledger = LoopbackLedger.Start();
        var logs = new RecordingLoggerProvider();
        using var loggerFactory = LoggerFactory.Create(builder => builder.SetMinimumLevel(LogLevel.Debug).AddProvider(logs));
        await using var fixture = LocalnetFixture.FromEndpoints(ledger.Endpoints, loggerFactory: loggerFactory);
        var darPath = Path.GetTempFileName();
        await File.WriteAllBytesAsync(darPath, [1, 2, 3]);

        try
        {
            await fixture.UploadDarAsync(darPath);
        }
        finally
        {
            File.Delete(darPath);
        }

        Assert.Contains(logs.Messages(AdminClientCategory), m => m.EndsWith(DiscoveryLogMessageSuffix, StringComparison.Ordinal));
    }

    [Fact]
    public async Task AllocatePartyAsync_logs_synchronizer_discovery_through_the_fixture_logger_factory()
    {
        using var ledger = LoopbackLedger.Start();
        var logs = new RecordingLoggerProvider();
        using var loggerFactory = LoggerFactory.Create(builder => builder.SetMinimumLevel(LogLevel.Debug).AddProvider(logs));
        await using var fixture = LocalnetFixture.FromEndpoints(ledger.Endpoints, loggerFactory: loggerFactory);

        await fixture.AllocatePartyAsync("globex");

        Assert.Contains(logs.Messages(AdminClientCategory), m => m.EndsWith(DiscoveryLogMessageSuffix, StringComparison.Ordinal));
    }

    [Fact]
    public async Task GetConnectedSynchronizersAsync_without_a_party_asks_for_the_participant_wide_list()
    {
        using var ledger = LoopbackLedger.Start();
        await using var fixture = LocalnetFixture.FromEndpoints(ledger.Endpoints);

        var synchronizers = await fixture.GetConnectedSynchronizersAsync();

        Assert.Equal(["global", "app-synchronizer"], synchronizers.Select(s => s.Alias));
        Assert.Equal(["/v2/state/connected-synchronizers"], ledger.RequestedPaths.Where(p => p.Contains("connected-synchronizers")));
    }

    [Fact]
    public async Task GetAppSynchronizerIdAsync_without_a_party_asks_for_the_participant_wide_list()
    {
        using var ledger = LoopbackLedger.Start();
        await using var fixture = LocalnetFixture.FromEndpoints(ledger.Endpoints);

        var id = await fixture.GetAppSynchronizerIdAsync();

        Assert.Equal("app-synchronizer::122b", id);
        Assert.Equal(["/v2/state/connected-synchronizers"], ledger.RequestedPaths.Where(p => p.Contains("connected-synchronizers")));
    }

    [Fact]
    public async Task ValidatorFixtureGetConnectedSynchronizersAsync_without_a_party_asks_for_the_participant_wide_list()
    {
        using var ledger = LoopbackLedger.Start();
        await using var fixture = LocalnetFixture.FromEndpoints(ledger.Endpoints);

        var synchronizers = await fixture.Validator(LocalnetProfile.AValidator1).GetConnectedSynchronizersAsync();

        Assert.Equal(["global", "app-synchronizer"], synchronizers.Select(s => s.Alias));
        Assert.Equal(["/v2/state/connected-synchronizers"], ledger.RequestedPaths.Where(p => p.Contains("connected-synchronizers")));
    }

    [Fact]
    public async Task ValidatorFixtureGetAppSynchronizerIdAsync_without_a_party_asks_for_the_participant_wide_list()
    {
        using var ledger = LoopbackLedger.Start();
        await using var fixture = LocalnetFixture.FromEndpoints(ledger.Endpoints);

        var id = await fixture.Validator(LocalnetProfile.AValidator1).GetAppSynchronizerIdAsync();

        Assert.Equal("app-synchronizer::122b", id);
        Assert.Equal(["/v2/state/connected-synchronizers"], ledger.RequestedPaths.Where(p => p.Contains("connected-synchronizers")));
    }

    private sealed class RecordingLoggerProvider : ILoggerProvider
    {
        private readonly ConcurrentQueue<(string Category, string Message)> _entries = new();

        public IEnumerable<string> Messages(string category) =>
            _entries.Where(e => e.Category == category).Select(e => e.Message).ToArray();

        public ILogger CreateLogger(string categoryName) => new Recorder(categoryName, _entries);

        public void Dispose()
        {
        }

        private sealed class Recorder(string category, ConcurrentQueue<(string Category, string Message)> sink) : ILogger
        {
            public IDisposable? BeginScope<TState>(TState state) where TState : notnull => null;

            public bool IsEnabled(LogLevel logLevel) => true;

            public void Log<TState>(
                LogLevel logLevel,
                EventId eventId,
                TState state,
                Exception? exception,
                Func<TState, Exception?, string> formatter) => sink.Enqueue((category, formatter(state, exception)));
        }
    }

    private sealed class LoopbackLedger : IDisposable
    {
        private const string SynchronizerId = "global::122a";

        private readonly HttpListener _listener = new();
        private readonly Task _serving;
        private readonly ConcurrentQueue<string> _requestedPaths = new();

        private LoopbackLedger(int port)
        {
            var root = $"http://127.0.0.1:{port}/";
            _listener.Prefixes.Add(root);
            _listener.Start();
            _serving = Task.Run(ServeAsync);
            Endpoints = new LocalnetEndpoints(
                JsonLedgerApi: new Uri(root),
                GrpcLedgerApi: new Uri("http://127.0.0.1:1/"),
                TokenEndpoint: new Uri(root + "token"),
                Audience: "aud",
                ClientId: "id",
                ClientSecret: "sec",
                Scope: null,
                ValidatorUserId: "user");
        }

        public LocalnetEndpoints Endpoints { get; }

        public IReadOnlyCollection<string> RequestedPaths => _requestedPaths;

        public static LoopbackLedger Start()
        {
            var probe = new TcpListener(IPAddress.Loopback, 0);
            probe.Start();
            var port = ((IPEndPoint)probe.LocalEndpoint).Port;
            probe.Stop();
            return new LoopbackLedger(port);
        }

        public void Dispose()
        {
            _listener.Close();
            _serving.Wait(TimeSpan.FromSeconds(5));
        }

        private async Task ServeAsync()
        {
            while (_listener.IsListening)
            {
                HttpListenerContext context;
                try
                {
                    context = await _listener.GetContextAsync().ConfigureAwait(false);
                }
                catch (Exception e) when (e is HttpListenerException or ObjectDisposedException)
                {
                    return;
                }

                var path = context.Request.Url!.AbsolutePath;
                _requestedPaths.Enqueue(context.Request.Url.PathAndQuery);
                var body = path switch
                {
                    "/token" => """{"access_token":"tok","token_type":"Bearer","expires_in":3600}""",
                    "/v2/state/connected-synchronizers" =>
                        $$"""{"connectedSynchronizers":[{"synchronizerAlias":"global","synchronizerId":"{{SynchronizerId}}"},{"synchronizerAlias":"app-synchronizer","synchronizerId":"app-synchronizer::122b"}]}""",
                    "/v2/parties" => """{"partyDetails":{"party":"globex-abc::ns","isLocal":true}}""",
                    _ => "{}",
                };
                var bytes = Encoding.UTF8.GetBytes(body);
                context.Response.ContentType = "application/json";
                context.Response.ContentLength64 = bytes.Length;
                await context.Response.OutputStream.WriteAsync(bytes).ConfigureAwait(false);
                context.Response.Close();
            }
        }
    }
}
