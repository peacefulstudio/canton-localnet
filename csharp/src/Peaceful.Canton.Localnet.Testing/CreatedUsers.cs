// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

namespace Peaceful.Canton.Localnet.Testing;

internal sealed class CreatedUsers
{
    private readonly object _gate = new();
    private readonly List<string> _outstanding = new();
    private readonly Dictionary<string, int> _attempts = new();

    public void Record(string userId)
    {
        lock (_gate)
        {
            _outstanding.Add(userId);
        }
    }

    public async Task DeleteAllAsync(UserBuilder builder)
    {
        string[] pending;
        Dictionary<string, int> attemptByUser;
        lock (_gate)
        {
            if (_outstanding.Count == 0)
            {
                return;
            }

            pending = _outstanding.ToArray();
            attemptByUser = new Dictionary<string, int>();
            foreach (var userId in pending)
            {
                _attempts[userId] = _attempts.GetValueOrDefault(userId) + 1;
                attemptByUser[userId] = _attempts[userId];
            }
        }

        var failures = new List<Exception>();
        foreach (var userId in pending)
        {
            try
            {
                await builder.DeleteAsync(userId, attemptByUser[userId], CancellationToken.None).ConfigureAwait(false);
                lock (_gate)
                {
                    _outstanding.Remove(userId);
                    _attempts.Remove(userId);
                }
            }
            catch (Exception exception)
            {
                failures.Add(exception);
            }
        }

        if (failures.Count == 1)
        {
            throw failures[0];
        }

        if (failures.Count > 1)
        {
            throw new AggregateException(
                $"{failures.Count} of {pending.Length} fixture-created ledger users could not be deleted.",
                failures);
        }
    }
}
