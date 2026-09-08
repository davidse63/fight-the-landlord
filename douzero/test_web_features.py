import unittest
from server import build_infoset


class PublicHistoryFeatures(unittest.TestCase):
    def test_passes_and_bombs_match_training_features(self):
        state = build_infoset({
            "position": "landlord_down", "hand": [4, 5],
            "card_play_action_seq": [[3, 3, 3, 3], [], [], [6]],
            "played_cards_landlord": [3, 3, 3, 3, 6],
            "last_move": [6], "last_move_position": "landlord",
            "num_cards_left": {"landlord": 15, "landlord_down": 2, "landlord_up": 17},
        })
        self.assertEqual(state.bomb_num, 1)
        self.assertEqual(state.last_two_moves, [[6], []])
        self.assertEqual(state.last_move_dict, {"landlord": [6], "landlord_down": [], "landlord_up": []})
        self.assertEqual(state.all_handcards, {"landlord_down": [4, 5]})


if __name__ == "__main__":
    unittest.main()
