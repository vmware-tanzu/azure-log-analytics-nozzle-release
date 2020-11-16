# How to contribute

We want to keep it as easy as possible to contribute changes that
get things working in your environment. There are a few guidelines that we
need contributors to follow so that we can have a chance of keeping on
top of things.

## Getting Started

* Make sure you have a [GitHub account](https://github.com/join).
* Submit a [GitHub issue](https://github.com/vmware-tanzu/nozzle-for-microsoft-azure-log-analytics/issues)
  if one does not already exist.
  * Clearly describe the issue including steps to reproduce when it is a bug.
  * Make sure you fill in the earliest version that you know has the issue.
* Fork the repository on GitHub.

## Making Changes

* Create a topic branch from where you want to base your work.
  * This is usually the main branch.
  * To quickly create a topic branch based on main, run `git checkout -b
    fix/main/my_contribution main`. Please avoid working directly on the
    `main` branch.
* Make commits of logical and atomic units.
* Check for unnecessary whitespace with `git diff --check` before committing.
* Make sure you have added the necessary tests for your changes.

## Submitting Changes

* Push your changes to a topic branch in your fork of the repository.
* Submit a pull request to this repository.
* Link pull request to your GitHub issue.

## Revert Policy

By running tests in advance and by engaging with peer review for prospective
changes, your contributions have a high probability of becoming long lived
parts of the the project. After being merged, the code will run through a
series of testing pipelines on a large number of operating system
environments. These pipelines can reveal incompatibilities that are difficult
to detect in advance.

If the code change results in a test failure, we will make our best effort to
correct the error. If a fix cannot be determined and committed in a reasonable
amount of time, the commit(s) responsible _may_ be reverted, at the
discretion of the committer and the repo maintainers. This action would be taken
to help maintain passing states in our testing pipelines.

The original contributor will be notified of the revert via the GitHub issue.

### Summary

* Create a linked issue and pull request with your proposed changes.
* Changes resulting in test pipeline failures will be reverted if they cannot
  be resolved within approximately one business day.

