Concept: A Fast, Intuitive, Native Terminal PR Reviewer

- Reviewing code on github UI is slow, non-intuitive and requires too many clicks.
- Checking PR status is too many clicks as well. Web is always going to be slower than terminal.
- Searching for mine or someone else's PRs is again slow. Searching for PR with PR number or a string is slow. Same with issues as well.


Found gh-dash to be nice, except it doesn't support line by line review.

Core principles:

1. Fast navigation
    - To PR/Issue/Branch
    - PRs/Issues assigned to me
    - PRs/Issues i am involved in
    - A feed for recent comments (can be tied to above maybe)
    - Vim keybindings (obviously).

2. Fast search
    - With PR/Issue number or a string against title.
    - Branch name
    - author name

3. Inline review comments
    - Probably the most challenging feature. 
    - View diff nicely, maybe with `delta`. 
    - Should be able to hit something like `c` on a line and be able to add comment on that line.

4. Fast repo switching across repositories in the current workspace.
    - Show repositories discovered under the current working directory.
    - Let me switch repos quickly from the dashboard instead of aggregating everything into one synthetic view.
    - Keep the model simple: selected repo drives the dashboard.

5. Aggressive caching and cache update.
    - Auto fetch latest PRs/comments/Issues/Diffs etc. in the background whenever opened.

6. Misc:
    - Edit your own PR desc, see commits, CI status
    - Approve/Request changes/Close/Reopen
    - UI hybrid of lazygit and gh-dash

7. Blazingly fast. Fast boot. Fast everything.
