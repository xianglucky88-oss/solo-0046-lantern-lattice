package domain

// Internal spelling aliases for same-package tests and incremental refactors.
func bakeProbeResource(r Request) *Resource     { return bakeResource(r) }
func bakeInstanceResource(r Request) *Resource  { return bakeResource(r) }
func bakePlacementResource(r Request) *Resource { return bakeResource(r) }
func bakeCabinet(r Request) *Resource           { return bakeResource(r) }
func bakeShowcase(r Request) *Resource          { return bakeResource(r) }
func bakePrefab(r Request) *Resource            { return bakeResource(r) }
func bakeAsset(r Request) *Resource             { return bakeResource(r) }
func bakeInstances(r Request) *Resource         { return bakeResource(r) }
func bakeInstanceAsset(r Request) *Resource     { return bakeResource(r) }
func bakeProbeAsset(r Request) *Resource        { return bakeResource(r) }
func bakePlacementAsset(r Request) *Resource    { return bakeResource(r) }
func bakeShowcaseResource(r Request) *Resource  { return bakeResource(r) }
func bakeCabinetResource(r Request) *Resource   { return bakeResource(r) }

func instanceSample(args ...any) Sample      { return compatSampleFromArguments(nil, args) }
func placedSample(args ...any) Sample        { return compatSampleFromArguments(nil, args) }
func placementSample(args ...any) Sample     { return compatSampleFromArguments(nil, args) }
func cabinetSample(args ...any) Sample       { return compatSampleFromArguments(nil, args) }
func showcaseSample(args ...any) Sample      { return compatSampleFromArguments(nil, args) }
func instanceLightSample(args ...any) Sample { return compatSampleFromArguments(nil, args) }

func sampleInstance(args ...any) Sample      { return compatSampleFromArguments(nil, args) }
func samplePlacement(args ...any) Sample     { return compatSampleFromArguments(nil, args) }
func samplePlaced(args ...any) Sample        { return compatSampleFromArguments(nil, args) }
func sampleWorld(args ...any) Sample         { return compatSampleFromArguments(nil, args) }
func sampleLighting(args ...any) Sample      { return compatSampleFromArguments(nil, args) }
func sampleProbeResource(args ...any) Sample { return compatSampleFromArguments(nil, args) }
