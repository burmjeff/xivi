# Channel matching

Xivi treats channel matching as an identity problem, not a semantic-similarity
problem. The automatic pipeline is deliberately precision-first:

1. Match a non-empty `tvg-id` after Unicode normalization, trimming, and case
   folding.
2. Canonicalize the displayed name (case, punctuation, quality/status tags,
   written numbers, and terminal Roman numerals).
3. Reject candidates with conflicting channel numbers, locale prefixes, US
   region suffixes, or east/west feeds.
4. Accept a unique canonical name. Otherwise rank with exact character-trigram
   Dice plus token Jaccard and require both `playlist.name_score` (default
   `0.96`) and a `0.05` lead over the runner-up.
5. Leave ambiguous results for the existing top-five suggestion UI.

Each committed relationship stores `match_method`, `match_score`,
`runner_up_score`, and `matcher_version`. Manual relationships are locked.
Deleting a relationship stores the normalized provider identity as a rejection,
so a later playlist refresh does not recreate it; manually adding it again
clears that rejection.

The database also enforces one source per playlist for each canonical channel.
Legacy vector tables and benchmark helpers remain, but playlist ingestion,
automatic matching, and the top-five suggestion endpoint no longer depend on
the embedding runtime.

## Why this is not bitwise matching

A bitset can make set intersection fast, but it does not decide whether two
channels mean the same thing. Hashing character bigrams into a small bitset can
also introduce collisions. Xivi uses exact, non-hashed n-gram sets for its small
candidate pools. If profiling later shows ranking is a bottleneck, postings or
bitsets can be added as a candidate-generation index without changing these
identity rules or the final score.

## Safe rollout

Keep `playlist.name_score` at `0.96` initially. Before changing it, sample the
persisted fuzzy matches, group false positives by `matcher_version`, and add
each newly discovered edge case to `backend/pkg/channelmatch/matcher_test.go`.
