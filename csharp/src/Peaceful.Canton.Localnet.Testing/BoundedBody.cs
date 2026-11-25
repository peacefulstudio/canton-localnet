// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

using System.Text;
using System.Text.Json;

namespace Peaceful.Canton.Localnet.Testing;

internal static class BoundedBody
{
    internal const int MaxReadBytes = 16 << 20;
    private const int MaxMessageCharacters = 512;
    private const string TruncationMarker = "...[truncated]";

    internal static async Task<string?> ReadWithinBoundAsync(
        HttpContent content,
        TimeSpan timeout,
        CancellationToken cancellationToken)
    {
        using var deadline = CancellationTokenSource.CreateLinkedTokenSource(cancellationToken);
        if (timeout != Timeout.InfiniteTimeSpan)
        {
            deadline.CancelAfter(timeout);
        }

        try
        {
            return await ReadWithinBoundAsync(content, deadline.Token).ConfigureAwait(false);
        }
        catch (OperationCanceledException e) when (!cancellationToken.IsCancellationRequested)
        {
            throw new TaskCanceledException(
                $"Reading the response body did not finish within the HttpClient.Timeout of {timeout}.",
                new TimeoutException(e.Message, e));
        }
    }

    private static async Task<string?> ReadWithinBoundAsync(HttpContent content, CancellationToken cancellationToken)
    {
        await using var stream = await content.ReadAsStreamAsync(cancellationToken).ConfigureAwait(false);
        var buffer = new byte[81_920];
        using var collected = new MemoryStream();
        while (true)
        {
            var wanted = (int)Math.Min(buffer.Length, MaxReadBytes + 1L - collected.Length);
            var read = await stream.ReadAsync(buffer.AsMemory(0, wanted), cancellationToken).ConfigureAwait(false);
            if (read == 0)
            {
                break;
            }
            collected.Write(buffer, 0, read);
            if (collected.Length > MaxReadBytes)
            {
                return null;
            }
        }
        return Encoding.UTF8.GetString(collected.GetBuffer(), 0, (int)collected.Length);
    }

    internal static string TooLargeMessage(string request, HttpResponseMessage response)
        => $"{request} returned {(int)response.StatusCode} {response.ReasonPhrase} with a body over the {MaxReadBytes}-byte bound.";

    internal static async Task<string> ReadAsync(
        HttpResponseMessage response,
        string request,
        TimeSpan timeout,
        CancellationToken cancellationToken)
        => await ReadWithinBoundAsync(response.Content, timeout, cancellationToken).ConfigureAwait(false)
            ?? throw new JsonLedgerApiException(TooLargeMessage(request, response), response.StatusCode, string.Empty);

    internal static async Task<T> ReadJsonAsync<T>(
        HttpResponseMessage response,
        string request,
        TimeSpan timeout,
        CancellationToken cancellationToken)
        where T : class
    {
        var body = await ReadAsync(response, request, timeout, cancellationToken).ConfigureAwait(false);
        return ParseJson<T>(body, response.StatusCode, response.ReasonPhrase, request);
    }

    internal static T ParseJson<T>(string body, System.Net.HttpStatusCode statusCode, string? reasonPhrase, string request)
        where T : class
    {
        T? payload;
        try
        {
            payload = JsonSerializer.Deserialize<T>(body, JsonSerializerOptions.Web);
        }
        catch (JsonException e)
        {
            throw new JsonLedgerApiException(
                $"{request} returned {(int)statusCode} {reasonPhrase} with a body that is not valid JSON: {ForMessage(body)}",
                statusCode,
                body,
                e);
        }

        return payload ?? throw new JsonLedgerApiException(
            $"{request} returned an empty body.",
            statusCode,
            body);
    }

    internal static string ForMessage(string body)
    {
        var trimmed = body.Trim();
        return trimmed.Length <= MaxMessageCharacters
            ? trimmed
            : string.Concat(trimmed.AsSpan(0, MaxMessageCharacters), TruncationMarker);
    }
}
