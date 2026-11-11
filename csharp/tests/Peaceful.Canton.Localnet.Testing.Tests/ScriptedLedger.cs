// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

using System.Net;
using System.Net.Sockets;
using System.Text;
using System.Text.Json;

namespace Peaceful.Canton.Localnet.Testing.Tests;

internal sealed record LedgerHit(string Method, string Path);

internal sealed record LedgerReply(int Status, string Body);

internal sealed class ScriptedLedger : IDisposable
{
    private const string RightsJson = """[{"kind":{"CanActAs":{"value":{"party":"alice::p"}}}}]""";

    private readonly HttpListener _listener = new();
    private readonly Task _serving;
    private readonly Func<string, int, LedgerReply> _onDelete;
    private readonly object _gate = new();
    private readonly List<LedgerHit> _hits = new();
    private readonly Dictionary<string, int> _deleteCalls = new();

    private ScriptedLedger(int port, Func<string, int, LedgerReply> onDelete)
    {
        _onDelete = onDelete;
        var root = $"http://127.0.0.1:{port}/";
        _listener.Prefixes.Add(root);
        _listener.Start();
        _serving = Task.Run(ServeAsync);
        Root = new Uri(root);
        Endpoints = new LocalnetEndpoints(
            JsonLedgerApi: Root,
            GrpcLedgerApi: new Uri("http://127.0.0.1:1/"),
            TokenEndpoint: new Uri(root + "token"),
            Audience: "aud",
            ClientId: "id",
            ClientSecret: "sec",
            Scope: null,
            ValidatorUserId: "user");
    }

    public Uri Root { get; }

    public LocalnetEndpoints Endpoints { get; }

    public IReadOnlyList<LedgerHit> Hits
    {
        get
        {
            lock (_gate)
            {
                return _hits.ToArray();
            }
        }
    }

    public IReadOnlyList<string> DeletedUserIds =>
        Hits.Where(hit => hit.Method == "DELETE").Select(hit => hit.Path["/v2/users/".Length..]).ToArray();

    public static ScriptedLedger Start(Func<string, int, LedgerReply>? onDelete = null)
    {
        var probe = new TcpListener(IPAddress.Loopback, 0);
        probe.Start();
        var port = ((IPEndPoint)probe.LocalEndpoint).Port;
        probe.Stop();
        return new ScriptedLedger(port, onDelete ?? ((_, _) => new LedgerReply(200, "{}")));
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

            var reply = await ReplyToAsync(context.Request).ConfigureAwait(false);
            var bytes = Encoding.UTF8.GetBytes(reply.Body);
            context.Response.StatusCode = reply.Status;
            context.Response.ContentType = "application/json";
            context.Response.ContentLength64 = bytes.Length;
            await context.Response.OutputStream.WriteAsync(bytes).ConfigureAwait(false);
            context.Response.Close();
        }
    }

    private async Task<LedgerReply> ReplyToAsync(HttpListenerRequest request)
    {
        var method = request.HttpMethod;
        var path = request.Url!.AbsolutePath;
        if (path == "/token")
        {
            return new LedgerReply(200, """{"access_token":"tok","token_type":"Bearer","expires_in":3600}""");
        }

        string requestBody;
        using (var reader = new StreamReader(request.InputStream, Encoding.UTF8))
        {
            requestBody = await reader.ReadToEndAsync().ConfigureAwait(false);
        }

        int deleteCall = 0;
        lock (_gate)
        {
            _hits.Add(new LedgerHit(method, path));
            if (method == "DELETE")
            {
                _deleteCalls[path] = deleteCall = _deleteCalls.GetValueOrDefault(path) + 1;
            }
        }

        return (method, path) switch
        {
            ("POST", "/v2/users") => new LedgerReply(200, CreatedUserJson(requestBody)),
            ("POST", _) when path.EndsWith("/rights", StringComparison.Ordinal) =>
                new LedgerReply(200, $$"""{"newlyGrantedRights":{{RightsJson}}}"""),
            ("PATCH", _) when path.EndsWith("/rights", StringComparison.Ordinal) =>
                new LedgerReply(200, $$"""{"newlyRevokedRights":{{RightsJson}}}"""),
            ("DELETE", _) => _onDelete(path["/v2/users/".Length..], deleteCall),
            _ => new LedgerReply(200, "{}"),
        };
    }

    private static string CreatedUserJson(string requestBody)
    {
        using var document = JsonDocument.Parse(requestBody);
        var id = document.RootElement.GetProperty("user").GetProperty("id").GetString();
        return JsonSerializer.Serialize(new { user = new { id } });
    }
}
