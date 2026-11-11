// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

using System.Text;
using Xunit;

namespace Peaceful.Canton.Localnet.Testing.Tests;

public class BoundedBodyTests
{
    [Fact]
    public async Task ReadAsync_returns_a_small_body_whole()
    {
        using var content = new StringContent("""{"cause":"nope"}""", Encoding.UTF8, "application/json");

        var body = await BoundedBody.ReadAsync(content, CancellationToken.None);

        Assert.Equal("""{"cause":"nope"}""", body);
    }

    [Fact]
    public async Task ReadAsync_stops_reading_at_sixteen_mebibytes()
    {
        using var content = new StreamContent(new EndlessXStream());

        var body = await BoundedBody.ReadAsync(content, CancellationToken.None);

        Assert.Equal(16_777_216, body.Length);
    }

    [Fact]
    public void ForMessage_keeps_a_512_character_body_whole()
    {
        var body = new string('y', 512);

        Assert.Equal(body, BoundedBody.ForMessage(body));
    }

    [Fact]
    public void ForMessage_cuts_a_longer_body_at_512_characters_and_marks_the_cut()
    {
        var body = new string('y', 513);

        Assert.Equal(new string('y', 512) + "...[truncated]", BoundedBody.ForMessage(body));
    }

    [Fact]
    public void ForMessage_trims_surrounding_whitespace()
    {
        Assert.Equal("""{"cause":"nope"}""", BoundedBody.ForMessage("  {\"cause\":\"nope\"}\n"));
    }

    private sealed class EndlessXStream : Stream
    {
        public override bool CanRead => true;
        public override bool CanSeek => false;
        public override bool CanWrite => false;
        public override long Length => throw new NotSupportedException();
        public override long Position { get => throw new NotSupportedException(); set => throw new NotSupportedException(); }

        public override int Read(byte[] buffer, int offset, int count)
        {
            Array.Fill(buffer, (byte)'x', offset, count);
            return count;
        }

        public override void Flush() => throw new NotSupportedException();
        public override long Seek(long offset, SeekOrigin origin) => throw new NotSupportedException();
        public override void SetLength(long value) => throw new NotSupportedException();
        public override void Write(byte[] buffer, int offset, int count) => throw new NotSupportedException();
    }
}
