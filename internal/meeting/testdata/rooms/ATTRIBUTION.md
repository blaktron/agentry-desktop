# Meetings in a room: test recordings

Five excerpts of the AMI Meeting Corpus, used to check how well speakers in
one room, recorded through one microphone, are told apart. Each is a 16 kHz
mono 16-bit WAV with a reference in RTTM: who speaks when, by the corpus's
own speaker ids, with times from the start of the excerpt.

| File | Meeting | Channel | From (s) | Length (s) | Voices |
|---|---|---|---|---|---|
| `two-voices` | ES2013c | Array1-01 | 2085 | 60 | 2 |
| `three-voices` | EN2003a | Array1-01 | 420 | 75 | 3 |
| `five-voices` | EN2001a | Array1-01 | 2775 | 120 | 5 |
| `crosstalk` | EN2002b | Array1-01 | 405 | 60 | 4, overlapping 60% of the time |
| `quiet-speaker` | IS1009d | Headset-0 to 3, mixed | 750 | 60 | 4, one 15 dB quieter |

`Array1-01` is one microphone of the table's array: the whole room, far from
every speaker. `quiet-speaker` is made from the four close-talking headsets,
summed with Headset-2 (speaker FIO084) lowered by 15 dB and the sum scaled to
leave headroom, because no single AMI room recording has one voice that much
quieter than the others. Nothing else was changed.

**The recordings** are from the AMI Meeting Corpus
(https://groups.inf.ed.ac.uk/ami/corpus/), released under the Creative
Commons Attribution 4.0 licence (https://creativecommons.org/licenses/by/4.0/).
Carletta, J. et al. (2005), "The AMI Meeting Corpus: A Pre-Announcement",
Machine Learning for Multimodal Interaction, pp. 28-39.

**The references** are cut from the word-level RTTM files of
BUTSpeechFIT/AMI-diarization-setup (https://github.com/BUTSpeechFIT/AMI-diarization-setup,
`only_words/rttms`), Apache License 2.0, which are made from the corpus's
own annotations, and shifted to the start of each excerpt.
