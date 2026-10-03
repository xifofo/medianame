import unittest

from release import validate


class ReleaseValidationTest(unittest.TestCase):
    repository = "xifofo/medianame"
    module = "github.com/xifofo/medianame"
    sha = "a" * 40

    def check_version(self, version, **overrides):
        args = {
            "version": version, "module": self.module, "repository": self.repository,
            "ref": "refs/heads/main", "sha": self.sha,
        }
        args.update(overrides)
        return validate(**args)

    def test_stable_and_prerelease_versions(self):
        for version in ["v0.1.0", "v1.0.0", "v1.20.3"]:
            with self.subTest(version=version):
                result = self.check_version(version)
                self.assertEqual(result, {"version": version, "module": self.module, "prerelease": "false"})
        for version in ["v0.1.0-rc.1", "v1.0.0-alpha", "v1.0.0-0", "v1.0.0-01a"]:
            with self.subTest(version=version):
                self.assertEqual(self.check_version(version)["prerelease"], "true")

    def test_invalid_versions_cannot_become_tags_or_outputs(self):
        for version in [
            "", "0.1.0", "v0.1", "v01.2.3", "v1.02.3", "v1.2.03", "v1.0.0-01",
            "v1.0.0-rc.01", "v1.0.0-rc..1", "v1.0.0+build", "v1.0.0\nmodule=other",
            "v1.0.0;echo injected", "v1.0.0/other", "v１.0.0", "v1.0.0 ",
        ]:
            with self.subTest(version=version), self.assertRaises(ValueError):
                self.check_version(version)

    def test_major_version_requires_matching_module_path(self):
        with self.assertRaises(ValueError):
            self.check_version("v2.0.0")
        self.check_version("v2.0.0", module=self.module + "/v2")
        with self.assertRaises(ValueError):
            self.check_version("v1.0.0", module=self.module + "/v2")
        with self.assertRaises(ValueError):
            self.check_version("v0.1.0", repository="someone/else")

    def test_only_main_and_full_commit_can_be_released(self):
        for ref in ["refs/heads/feature", "refs/tags/v0.1.0", "main"]:
            with self.subTest(ref=ref), self.assertRaises(ValueError):
                self.check_version("v0.1.0", ref=ref)
        for sha in ["main", "1234567", "g" * 40]:
            with self.subTest(sha=sha), self.assertRaises(ValueError):
                self.check_version("v0.1.0", sha=sha)

    def test_retry_can_only_reuse_tag_at_same_commit(self):
        self.check_version("v0.1.0", existing_commit=self.sha)
        with self.assertRaises(ValueError):
            self.check_version("v0.1.0", existing_commit="b" * 40)


if __name__ == "__main__":
    unittest.main()
