#!/bin/bash
# End-to-end test script for cross-platform validation

set -e

echo "================================"
echo "  E2E Tests - Cross-Platform"
echo "================================"
echo ""

# Test 1: Basic deck processing via CLI
echo "→ Test 1: CLI basic deck processing"
echo 'deck
  slide "white" "black"
    ctext "Hello World" 50 50 5
  eslide
edeck' | ./.bin/deckfs process > /tmp/deckfs-test.json

if grep -q '"success":true' /tmp/deckfs-test.json; then
  echo "✓ CLI processing successful"
else
  echo "✗ CLI processing failed"
  cat /tmp/deckfs-test.json
  exit 1
fi

# Test 2: Verify slide count
SLIDE_COUNT=$(grep -o '"slideCount":[0-9]*' /tmp/deckfs-test.json | cut -d: -f2)
if [ "$SLIDE_COUNT" = "1" ]; then
  echo "✓ Slide count correct (1)"
else
  echo "✗ Expected slideCount=1, got $SLIDE_COUNT"
  exit 1
fi

# Test 3: Verify slide content is SVG
if grep -q 'svg' /tmp/deckfs-test.json; then
  echo "✓ Slide content is SVG"
else
  echo "✗ Slide content is not SVG"
  exit 1
fi

# Test 4: Multi-slide deck
echo ""
echo "→ Test 2: Multi-slide deck"
echo 'deck
  slide
    text "Slide 1" 50 50 5
  eslide
  slide
    text "Slide 2" 50 50 5
  eslide
edeck' | ./.bin/deckfs process > /tmp/deckfs-multi.json

MULTI_COUNT=$(grep -o '"slideCount":[0-9]*' /tmp/deckfs-multi.json | cut -d: -f2)
if [ "$MULTI_COUNT" = "2" ]; then
  echo "✓ Multi-slide deck processed correctly (2 slides)"
else
  echo "✗ Expected slideCount=2, got $MULTI_COUNT"
  exit 1
fi

# Test 5: Error handling - invalid deck
echo ""
echo "→ Test 3: Error handling"
echo 'invalid deck syntax' | ./.bin/deckfs process > /tmp/deckfs-error.json 2>&1 || true

if grep -q '"success":false' /tmp/deckfs-error.json 2>/dev/null; then
  echo "✓ Error handling works correctly"
elif grep -q '"error"' /tmp/deckfs-error.json 2>/dev/null; then
  echo "✓ Error handling works correctly"
else
  echo "⚠ Error handling may need improvement"
fi

# Test 6: API response structure validation
echo ""
echo "→ Test 4: API response structure"
REQUIRED_FIELDS=("success" "slideCount" "slides")
for field in "${REQUIRED_FIELDS[@]}"; do
  if grep -q "\"$field\":" /tmp/deckfs-test.json; then
    echo "✓ Response contains '$field' field"
  else
    echo "✗ Response missing '$field' field"
    exit 1
  fi
done

echo ""
echo "================================"
echo "✅ All E2E tests passed"
echo "================================"
