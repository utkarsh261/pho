# Custom tabs

Add your own tabs to the dashboard's Pull requests panel, each a GitHub search. They show up after the built-in tabs, in the order you list them, in pho's config (`~/.config/pho/config.toml`):

```toml
[dashboard]
default_tab = "Team"   # optional: open on this tab

[[dashboard.tabs]]
name = "Team"
query = "author:alice author:bob author:carol"

[[dashboard.tabs]]
name = "My merged"
short = "Merged"       # shown when the panel is too narrow for full names
query = "author:@me is:merged"

[[dashboard.tabs]]
name = "Docs"
query = "label:docs draft:false"
```

## Queries

`query` takes anything GitHub's [PR search](https://docs.github.com/en/search-github/searching-on-github/searching-issues-and-pull-requests) does, such as `author:`, `label:`, `review-requested:`, `reviewed-by:` or `base:`. `@me` means you. Repeating a qualifier like `author:` matches any of them.

- A tab always searches the repo selected in pho, so leave out `repo:`, `org:` and `user:`.
- Tabs show open PRs unless the query picks a state itself (`is:merged`, `is:closed`, `state:`, `merged:>2026-01-01`…).
- `OR`, `AND` and parentheses aren't supported by GitHub's API search; repeat a qualifier instead.
- A tab shows up to 100 PRs, newest activity first.

## Names

`name` is shown in the tab bar and must differ from the built-in tabs and from other custom tabs. `short` is optional and is cut to 8 characters; without it, the name is cut short when space is tight.

`default_tab` takes a custom tab's name or a built-in tab: `my_prs`, `needs_review`, `involving`, `all` or `recent`.

You can have up to 5 custom tabs.

## Loading

A custom tab runs its search only while it's on screen, so tabs you don't open cost nothing. Results are cached the same way as the other tabs: switching back to a tab shows them straight away and refreshes them in the background once they're a couple of minutes old. `R` refreshes the dashboard and the tab you're on.

If GitHub rejects a search, the tab shows its error. A qualifier GitHub doesn't recognise usually just finds nothing, so an unexpectedly empty tab is worth a look at its query.
