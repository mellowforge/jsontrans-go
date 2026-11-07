import unittest
from summarize import parse


class ParseTests(unittest.TestCase):
    def test_units_and_samples(self):
        text = ("BenchmarkX/a-1 10 120 ns/op 3 MB/s 40 B/op 2 allocs/op\nPASS\n"
                "BenchmarkX/a-1 10 100 ns/op 3 MB/s 20 B/op 1 allocs/op\nPASS\n")
        value = parse(text, 2)["BenchmarkX/a"]
        self.assertEqual(value["ns/op"], {"median": 110, "min": 100, "max": 120})
        self.assertEqual(value["B/op"]["median"], 30)
        self.assertEqual(value["allocs/op"]["median"], 1.5)

    def test_missing_and_duplicate_samples_are_rejected(self):
        sample = "BenchmarkX/a 1 100 ns/op 20 B/op 1 allocs/op\n"
        for text in [sample, "PASS\n", sample + "PASS\nPASS\n", sample * 2 + "PASS\n"]:
            with self.subTest(text=text), self.assertRaises(ValueError):
                parse(text, 1)


if __name__ == "__main__":
    unittest.main()
