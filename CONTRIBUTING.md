# Contributing to This Repository

Oracle welcomes your contributions! There are multiple ways that you can contribute.

## Opening issues

For bugs or enhancement requests, please file a GitHub issue, unless the problem is security-related.

When filing a bug, remember that the more specific the bug is, the more likely it is to be fixed. If you think you've found a security vulnerability, then do not raise a GitHub issue. Instead, follow the instructions in our [security policy](./SECURITY.md).

## Contributing code

Oracle welcomes your code contributions. Before submitting code by using a pull request,
you must sign the [Oracle Contributor Agreement][OCA] (OCA), and your commits must include the following line, using the name and e-mail address you used to sign the OCA:

```text
Signed-off-by: Your Name <you@example.org>
```

You can add this line automatically to pull requests by committing with `--sign-off`
or `-s`. For example:

```text
git commit --signoff
```

Only pull requests from committers that can be verified as having signed the OCA
can be accepted.

## Pull request process

1. Ensure that there is an issue created to track and discuss the fix or enhancement that you intend to submit.
1. Fork this repository.
1. Create a branch in your fork to implement the changes. Oracle recommends using
   the issue number as part of your branch name. For example: `1234-fixes`
1. Ensure that any documentation is updated with the changes that are required
   by your change.
1. If the base image has been changed, then ensure that any examples are updated. 
1. Submit the pull request. *Do not leave the pull request blank*. Explain exactly
   what your changes are meant to do, and provide simple steps to indicate how to validate
   your changes. Ensure that you reference the issue that you created as well.
1. Before the changes are merged, Oracle will assign the pull request to two or three people for review. 

## Code of conduct

Follow the [Golden Rule](https://en.wikipedia.org/wiki/Golden_Rule). If you'd
like more specific guidelines, then see the [Contributor Covenant Code of Conduct][COC].

[OCA]: https://oca.opensource.oracle.com
[COC]: https://www.contributor-covenant.org/version/1/4/code-of-conduct/
