package clipdemo

import "testing"

func TestCosineSimilarity(t *testing.T) {
	score, err := CosineSimilarity([]float32{1, 0}, []float32{1, 1})
	if err != nil {
		t.Fatal(err)
	}
	if score < 0.707 || score > 0.708 {
		t.Fatalf("score = %f", score)
	}
}

func TestCosineSimilarityRejectsInvalidVectors(t *testing.T) {
	if _, err := CosineSimilarity([]float32{1}, []float32{1, 2}); err == nil {
		t.Fatal("expected dimension error")
	}
}
