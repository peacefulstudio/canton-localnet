// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

using Microsoft.Extensions.Logging;

namespace Peaceful.Canton.Localnet.Testing;

internal static class LedgerReadRetry
{
    internal static async Task<T> RunAsync<T>(
        string description,
        Func<CancellationToken, Task<T>> read,
        Func<T, bool> isIncomplete,
        DarUploaderRetryOptions retryOptions,
        ILogger logger,
        CancellationToken cancellationToken)
    {
        for (var attempt = 1; ; attempt++)
        {
            string outcome;
            try
            {
                var result = await read(cancellationToken).ConfigureAwait(false);
                if (!isIncomplete(result) || attempt >= retryOptions.MaxAttempts)
                {
                    return result;
                }
                outcome = "reported no connected synchronizer";
            }
            catch (JsonLedgerApiException e)
                when (TransientHttpStatus.IsTransient(e.StatusCode, e.ResponseBody) && attempt < retryOptions.MaxAttempts)
            {
                outcome = $"returned {(int)e.StatusCode}";
            }
            catch (TaskCanceledException timeout) when (!cancellationToken.IsCancellationRequested)
            {
                if (attempt >= retryOptions.MaxAttempts)
                {
                    throw new TimeoutException($"{description} timed out on all {retryOptions.MaxAttempts} attempts.", timeout);
                }
                outcome = "timed out";
            }

            var delay = retryOptions.DelayForAttempt(attempt);
            logger.LogWarning(
                "{Description} {Outcome} (attempt {Attempt}/{MaxAttempts}); retrying in {Delay}.",
                description, outcome, attempt, retryOptions.MaxAttempts, delay);
            await retryOptions.Delay(delay, cancellationToken).ConfigureAwait(false);
        }
    }

    internal static Task<T> RunAsync<T>(
        string description,
        Func<CancellationToken, Task<T>> read,
        DarUploaderRetryOptions retryOptions,
        ILogger logger,
        CancellationToken cancellationToken)
        => RunAsync(description, read, static _ => false, retryOptions, logger, cancellationToken);
}
