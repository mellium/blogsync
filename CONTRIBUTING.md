# Contributing

## Issues

Bugs and feature requests can be started by opening an [issue][issues].
Always open an issue before creating a pull request unless the patch is trivial,
all patches should reference an issue and should generally only contain a single
logical change.

Once you file an issue or find an existing issue, make sure to mention that
you're working on the problem and outline your plans so that someone else
doesn't duplicate your work.


## Creating Patches

When you create your commit, be sure to follow convention for the commit message
and code formatting.

  - Format all code with `go fmt`
  - Write documentation comments for any new public identifiers
  - Write tests for your code
  - Follow Go best practices
  - Write a detailed commit message
  - Submit a patch and wait for review

Commit messages should start with the name of the Go package being modified,
followed by a colon or the string "all" if it affects the entire module.
The rest of the first line should be a short description of how it modifies the
project, for example, the following is a good first line for a commit message:

    dial: fix flaky tests

After the first line should be a blank line, followed by a paragraph or so
describing the change in more detail.
This provides context for the commit and should be written in full sentences.
Do not use Markdown, HTML, or other formatting in your commit messages.

For example, a good full commit message might be:

    dial: fix flaky tests

    Previously a DNS request might have been made for A or AAAA records
    depending on what networks were available. Tests expected AAAA requests
    so they would fail on machines that only had IPv4 networking.

To submit your patch, first learn to use `git send-email` by reading
[git-send-email.io], then read the SourceHut [mailing list etiquette] guide.
You can send patches to my general purpose patches [mailing list].

Please prefix the subject with `[PATCH blogsync]`.
To configure your checkout of this repo to always use the correct prefix and
send to the correct list cd into the repo and run:

    git config sendemail.to ~samwhited/patches@lists.sr.ht
    git config format.subjectPrefix 'PATCH blogsync'

[git-send-email.io]: https://git-send-email.io/
[mailing list etiquette]: https://man.sr.ht/lists.sr.ht/etiquette.md
[mailing list]: https://lists.sr.ht/~samwhited/patches

Once your patch is submitted, you will hear back from a maintainer within 5
days.
If you haven't heard back by then, feel free to ping the list to move it back to
the top of the maintainers inbox.


## License

The package may be used under the terms of the BSD 2-Clause License a copy of
which may be found in the file "[LICENSE]".

Unless you explicitly state otherwise, any contribution submitted for inclusion
in the work by you shall be licensed as above, without any additional terms or
conditions.


[issues]: https://todo.sr.ht/~samwhited/blogsync
[pull requests]: https://github.com/mellium/xmpp/pulls?q=is%3Apr
[LICENSE]: ./LICENSE
