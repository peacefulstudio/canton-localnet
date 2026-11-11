// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

using System.Text;

namespace Peaceful.Canton.Localnet.Testing;

internal static class BoundedBody
{
    private const int MaxReadBytes = 16 << 20;
    private const int MaxMessageCharacters = 512;
    private const string TruncationMarker = "...[truncated]";

    internal static async Task<string> ReadAsync(HttpContent content, CancellationToken cancellationToken)
    {
        await using var stream = await content.ReadAsStreamAsync(cancellationToken).ConfigureAwait(false);
        var buffer = new byte[Math.Min(MaxReadBytes, 81_920)];
        using var collected = new MemoryStream();
        while (collected.Length < MaxReadBytes)
        {
            var wanted = (int)Math.Min(buffer.Length, MaxReadBytes - collected.Length);
            var read = await stream.ReadAsync(buffer.AsMemory(0, wanted), cancellationToken).ConfigureAwait(false);
            if (read == 0)
            {
                break;
            }
            collected.Write(buffer, 0, read);
        }
        return Encoding.UTF8.GetString(collected.GetBuffer(), 0, (int)collected.Length);
    }

    internal static string ForMessage(string body)
    {
        var trimmed = body.Trim();
        return trimmed.Length <= MaxMessageCharacters
            ? trimmed
            : string.Concat(trimmed.AsSpan(0, MaxMessageCharacters), TruncationMarker);
    }
}
