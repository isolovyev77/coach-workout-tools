#!/usr/bin/env python3
import importlib.util
import pathlib
import unittest


MODULE_PATH = pathlib.Path(__file__).with_name("build_trenda_payload.py")
SPEC = importlib.util.spec_from_file_location("build_trenda_payload", MODULE_PATH)
BUILDER = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(BUILDER)


class BuildTrendaPayloadTest(unittest.TestCase):
    def test_preserves_base_sections_and_builds_one_complex(self):
        plan = {
            "workout_id": 65934,
            "title": "",
            "description": "",
            "warmup": [
                {
                    "kind": "text",
                    "title": "",
                    "lines": ["10 scap pull-ups", "3 toes-to-bars"],
                }
            ],
            "workout": [
                {
                    "kind": "mixed_modal",
                    "title": "AMRAP 12 mins: Double Unders and Toes-to-bars",
                    "lines": [
                        "Complete as many rounds as possible in 12 mins of:",
                        "18 Double Unders",
                        "9 Toes-to-bars",
                        "*every 2mins (starting at 0:00) complete 12 box jumps 60cm",
                    ],
                    "instructions_lines": [
                        "Каждые 2 минуты (начиная с 0:00) выполните:",
                        "12 прыжков на коробку, 60 см",
                        "Продолжайте с того места, где остановились перед каждым набором прыжков на коробку.",
                    ],
                }
            ],
            "cooldown": [],
        }

        BUILDER.validate_plan(plan, "update-body")
        payload = BUILDER.build_payload(plan, "update-body")

        self.assertEqual(payload["title"], "")
        self.assertEqual(payload["cooldown"], [])
        self.assertEqual(payload["warmup"][0]["exercises"][0]["title"], "")
        self.assertEqual(len(payload["workout"]), 1)

        complex_exercise = payload["workout"][0]["exercises"][0]
        self.assertEqual(complex_exercise["type"], "MixedModal")
        self.assertIn("18 Double Unders", complex_exercise["complex_details"])
        self.assertIn("Каждые 2 минуты", complex_exercise["description"])
        self.assertNotIn("exercise_id", complex_exercise)
        self.assertNotIn("sets", complex_exercise)
        self.assertNotIn("video_urls", complex_exercise)

    def test_rejects_non_empty_day_title(self):
        plan = {
            "workout_id": 65934,
            "title": "NATRIUM Intermediate",
            "warmup": [],
            "workout": [],
            "cooldown": [],
        }

        with self.assertRaisesRegex(ValueError, "title must be empty"):
            BUILDER.validate_plan(plan, "update-body")

    def test_rejects_named_base_warmup(self):
        plan = {
            "workout_id": 65934,
            "title": "",
            "warmup": [
                {
                    "kind": "text",
                    "title": "Warm-up for the WOD",
                    "lines": ["Bike 3 min"],
                }
            ],
            "workout": [],
            "cooldown": [],
        }

        with self.assertRaisesRegex(ValueError, "warmup text block title must be empty"):
            BUILDER.validate_plan(plan, "update-body")

    def test_rejects_btwb_link_without_explicit_permission(self):
        plan = {
            "workout_id": 65934,
            "title": "",
            "description": "Source: https://btwb.com/workouts/123",
            "warmup": [],
            "workout": [],
            "cooldown": [],
        }

        with self.assertRaisesRegex(ValueError, "BTWB attribution"):
            BUILDER.validate_plan(plan, "update-body")

    def test_alternating_skill_work_builds_linked_a1_a2_with_three_sets(self):
        plan = {
            "workout_id": 65934,
            "title": "",
            "warmup": [],
            "workout": [
                {
                    "kind": "superset",
                    "exercises": [
                        {
                            "kind": "exercise",
                            "title": "Scapular Pull-Up",
                            "exercise_id": 101,
                            "sets": [{"reps": 8}, {"reps": 8}, {"reps": 8}],
                            "description_lines": ["отдых до конца 2:00"],
                        },
                        {
                            "kind": "exercise",
                            "title": "Kipping Swing",
                            "exercise_id": 202,
                            "sets": [{"reps": 10}, {"reps": 10}, {"reps": 10}],
                            "description_lines": ["отдых 0:30"],
                        },
                    ],
                }
            ],
            "cooldown": [],
        }

        BUILDER.validate_plan(plan, "update-body")
        payload = BUILDER.build_payload(plan, "update-body")
        block = payload["workout"][0]

        self.assertTrue(block["is_superset"])
        self.assertEqual([exercise["sort_order"] for exercise in block["exercises"]], [0, 1])
        self.assertEqual([len(exercise["sets"]) for exercise in block["exercises"]], [3, 3])
        self.assertIn("отдых до конца 2:00", block["exercises"][0]["description"])
        self.assertIn("отдых 0:30", block["exercises"][1]["description"])


class RegressionGuards(unittest.TestCase):
    """Случаи, где сборщик раньше молча портил тело запроса."""

    def base_plan(self, **extra):
        plan = {"workout_id": 65934, "warmup": [], "workout": [], "cooldown": []}
        plan.update(extra)
        return plan

    def test_unknown_set_key_raises(self):
        plan = self.base_plan(workout=[{
            "kind": "exercise", "title": "Приседания",
            "sets": [{"weight": 60, "repetitions": 5}],
        }])
        with self.assertRaises(ValueError) as ctx:
            BUILDER.build_payload(plan, "update-body")
        self.assertIn("неизвестные ключи подхода", str(ctx.exception))

    def test_set_without_load_raises(self):
        plan = self.base_plan(workout=[{
            "kind": "exercise", "title": "Приседания", "sets": [{"order": 1}],
        }])
        with self.assertRaises(ValueError):
            BUILDER.build_payload(plan, "update-body")

    def test_nested_superset_raises_instead_of_dropping(self):
        plan = self.base_plan(workout=[{
            "kind": "superset",
            "exercises": [{"kind": "superset", "exercises": [
                {"kind": "exercise", "title": "A", "sets": [{"reps": 5}]},
                {"kind": "exercise", "title": "B", "sets": [{"reps": 5}]},
            ]}],
        }])
        with self.assertRaises(ValueError) as ctx:
            BUILDER.build_payload(plan, "update-body")
        self.assertIn("superset", str(ctx.exception))

    def test_missing_kind_gives_readable_error(self):
        plan = self.base_plan(workout=[{"title": "без kind"}])
        with self.assertRaises(ValueError) as ctx:
            BUILDER.validate_plan(plan, "update-body")
        self.assertIn("kind", str(ctx.exception))

    def test_text_block_in_workout_is_not_cooldown(self):
        plan = self.base_plan(workout=[{"kind": "text", "title": "", "lines": ["4 мин техники"]}])
        payload = BUILDER.build_payload(plan, "update-body")
        self.assertEqual(payload["workout"][0]["exercises"][0]["type"], "Exercise")

    def test_title_preserved_when_allowed(self):
        plan = self.base_plan(title="Нижняя часть тела", allow_title=True)
        BUILDER.validate_plan(plan, "update-body")
        payload = BUILDER.build_payload(plan, "update-body")
        self.assertEqual(payload["title"], "Нижняя часть тела")

    def test_title_rejected_without_flag(self):
        plan = self.base_plan(title="NATRIUM Intermediate")
        with self.assertRaises(ValueError):
            BUILDER.validate_plan(plan, "update-body")


if __name__ == "__main__":
    unittest.main()
