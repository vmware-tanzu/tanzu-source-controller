# Contributing to tanzu-source-controller

The tanzu-source-controller project team welcomes contributions from the community. If you wish to contribute code and you have not signed our contributor license agreement (CLA), our bot will update the issue when you open a Pull Request. 

For any questions about the CLA process, please refer to our [FAQ](https://cla.vmware.com/faq). Your signature certifies that you wrote the patch or have the right to pass it on as an open-source patch.

`Signed-off-by: Jane Doe <jane.doe@example.com>`

The signature must contain your real name (sorry, no pseudonyms or anonymous contributions) If your `user.name` and `user.email` are configured in your Git config, you can sign your commit automatically with `git commit -s`.

## Ways to contribute

We welcome many different types of contributions and not all of them need a Pull request. Contributions may include:

* New features and proposals
* Documentation
* Bug fixes
* Issue Triage
* Answering questions and giving feedback
* Helping to onboard new contributors
* Other related activities

## Getting started

### Development Environment Setup

Follow the documentation in the [development document](./docs/development.md) to get started with developing Tanzu Source Controller.

## Contribution Flow

This is a rough outline of what a contributor's workflow looks like:

- Make a fork of the repository within your GitHub account
- Create a topic branch in your fork from where you want to base your work
- Make commits of logical units
- Make sure your commit messages are with the proper format, quality and descriptiveness (see below)
- Push your changes to the topic branch in your fork
- Create a pull request containing that commit

## Acceptance policy

These things will make a PR more likely to be accepted:

- a well-described requirement
- tests for new code
- tests for old code!
- new code and tests follow the conventions in old code and tests
- a good commit message (see below)
- all code must abide [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments)
- names should abide [What's in a name](https://talks.golang.org/2014/names.slide#1)
- code should have appropriate test coverage and tests should be written
  to work with `go test`

In general, we will merge a PR once one maintainer has endorsed it.
For substantial changes, more people may become involved, and you might
get asked to resubmit the PR or divide the changes into more than one PR.

### Formatting Commit Messages

We follow the conventions on [How to Write a Git Commit Message](http://chris.beams.io/posts/git-commit/).

Be sure to include any related GitHub issue references in the commit message.  See
[GFM syntax](https://guides.github.com/features/mastering-markdown/#GitHub-flavored-markdown) for referencing issues
and commits.

## Reporting Bugs and Creating Issues

When opening a new issue, try to roughly follow the commit message format conventions above.
