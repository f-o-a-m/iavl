package server

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/cosmos/iavl"
	pb "github.com/cosmos/iavl/proto"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/pkg/errors"
	dbm "github.com/tendermint/tm-db"
)

var _ pb.IAVLServiceServer = (*IAVLServer)(nil)

// IAVLServer implements the gRPC IAVLServiceServer interface. It provides a gRPC
// API over an IAVL tree.
type IAVLServer struct {
	tree *iavl.MutableTree
}

// New creates an IAVLServer.
func New(db dbm.DB, cacheSize, version int64, keepEvery int64, keepRecent int64) (*IAVLServer, error) {
	opts := &iavl.Options{
		KeepEvery:  keepEvery,
		KeepRecent: keepRecent,
	}
	tree, err := iavl.NewMutableTreeWithOpts(db, dbm.NewMemDB(), int(cacheSize), opts)
	if err != nil {
		return nil, errors.Wrap(err, "unable to create iavl tree")
	}

	if _, err := tree.LoadVersion(version); err != nil {
		return nil, errors.Wrapf(err, "unable to load version %d", version)
	}

	return &IAVLServer{tree: tree}, nil
}

// Has returns a result containing a boolean on whether or not the IAVL tree
// has a given key at a specific tree version.
func (s *IAVLServer) Has(_ context.Context, req *pb.HasRequest) (*pb.HasResponse, error) {
	if !s.tree.VersionExists(req.Version) {
		return nil, iavl.ErrVersionDoesNotExist
	}

	iTree, err := s.tree.GetImmutable(req.Version)
	if err != nil {
		return nil, err
	}

	return &pb.HasResponse{Result: iTree.Has(req.Key)}, nil
}

// Get returns a result containing the IAVL tree version and value for a given
// key based on the current state (version) of the tree.
func (s *IAVLServer) Get(_ context.Context, req *pb.GetRequest) (*pb.GetResponse, error) {
	idx, value := s.tree.Get(req.Key)
	if value == nil {
		s := status.New(codes.NotFound, "the key requested does not exist")
		return nil, s.Err()
	}

	return &pb.GetResponse{Index: idx, Value: value}, nil
}

// GetWithProof returns a result containing the IAVL tree version and value for
// a given key based on the current state (version) of the tree including a
// verifiable Merkle proof.
func (s *IAVLServer) GetWithProof(ctx context.Context, req *pb.GetRequest) (*pb.GetWithProofResponse, error) {
	value, proof, err := s.tree.GetWithProof(req.Key)
	if err != nil {
		return nil, err
	}

	if value == nil {
		s := status.New(codes.NotFound, "the key requested does not exist")
		return nil, s.Err()
	}

	proofPb := iavl.ConvertRangeProofToProto(req.Key, proof)

	return &pb.GetWithProofResponse{Value: value, Proof: proofPb}, nil
}

// GetVersioned returns a result containing the IAVL tree version and value
// for a given key at a specific tree version.
func (s *IAVLServer) GetVersioned(_ context.Context, req *pb.GetVersionedRequest) (*pb.GetResponse, error) {
	if !s.tree.VersionExists(req.Version) {
		return nil, iavl.ErrVersionDoesNotExist
	}

	iTree, err := s.tree.GetImmutable(req.Version)
	if err != nil {
		return nil, err
	}

	idx, value := iTree.Get(req.Key)

	return &pb.GetResponse{Index: idx, Value: value}, nil
}

// GetVersionedWithProof returns a result containing the IAVL tree version and
// value for a given key at a specific tree version including a verifiable Merkle
// proof.
func (s *IAVLServer) GetVersionedWithProof(_ context.Context, req *pb.GetVersionedRequest) (*pb.GetWithProofResponse, error) {
	value, proof, err := s.tree.GetVersionedWithProof(req.Key, req.Version)
	if err != nil {
		return nil, err
	}

	if value == nil {
		s := status.New(codes.NotFound, "the key requested does not exist")
		return nil, s.Err()
	}

	proofPb := iavl.ConvertRangeProofToProto(req.Key, proof)

	return &pb.GetWithProofResponse{Value: value, Proof: proofPb}, nil
}

// Set returns a result after inserting a key/value pair into the IAVL tree
// based on the current state (version) of the tree.
func (s *IAVLServer) Set(_ context.Context, req *pb.SetRequest) (*pb.SetResponse, error) {
	if req.Key == nil {
		return nil, errors.New("key cannot be nil")
	}

	if req.Value == nil {
		return nil, errors.New("value cannot be nil")
	}

	return &pb.SetResponse{Result: s.tree.Set(req.Key, req.Value)}, nil
}

// Remove returns a result after removing a key/value pair from the IAVL tree
// based on the current state (version) of the tree.
func (s *IAVLServer) Remove(_ context.Context, req *pb.RemoveRequest) (*pb.RemoveResponse, error) {
	value, removed := s.tree.Remove(req.Key)
	return &pb.RemoveResponse{Value: value, Removed: removed}, nil
}

// SaveVersion saves a new IAVL tree version to the DB based on the current
// state (version) of the tree. It returns a result containing the hash and
// new version number.
func (s *IAVLServer) SaveVersion(_ context.Context, _ *empty.Empty) (*pb.SaveVersionResponse, error) {
	root, version, err := s.tree.SaveVersion()
	if err != nil {
		return nil, err
	}

	return &pb.SaveVersionResponse{RootHash: root, Version: version}, nil
}

// DeleteVersion deletes an IAVL tree version from the DB. The version can then
// no longer be accessed. It returns a result containing the version and root
// hash of the versioned tree that was deleted.
func (s *IAVLServer) DeleteVersion(_ context.Context, req *pb.DeleteVersionRequest) (*pb.DeleteVersionResponse, error) {
	iTree, err := s.tree.GetImmutable(req.Version)
	if err != nil {
		return nil, err
	}

	if err := s.tree.DeleteVersion(req.Version); err != nil {
		return nil, err
	}

	return &pb.DeleteVersionResponse{RootHash: iTree.Hash(), Version: req.Version}, nil
}

// Version returns the IAVL tree version based on the current state.
func (s *IAVLServer) Version(_ context.Context, _ *empty.Empty) (*pb.VersionResponse, error) {
	return &pb.VersionResponse{Version: s.tree.Version()}, nil
}

// Hash returns the IAVL tree root hash based on the current state.
func (s *IAVLServer) Hash(_ context.Context, _ *empty.Empty) (*pb.HashResponse, error) {
	return &pb.HashResponse{RootHash: s.tree.Hash()}, nil
}

// VersionExists returns a result containing a boolean on whether or not a given
// version exists in the IAVL tree.
func (s *IAVLServer) VersionExists(_ context.Context, req *pb.VersionExistsRequest) (*pb.VersionExistsResponse, error) {
	return &pb.VersionExistsResponse{Result: s.tree.VersionExists(req.Version)}, nil
}

// Verify verifies an IAVL range proof returning an error if the proof is invalid.
func (*IAVLServer) Verify(ctx context.Context, req *pb.VerifyRequest) (*empty.Empty, error) {
	proof := iavl.ConvertProtoRangeProof(req.Proof)
	if err := proof.Verify(req.RootHash); err != nil {
		return nil, err
	}

	return &empty.Empty{}, nil
}

// VerifyItem verifies if a given key/value pair in an IAVL range proof returning
// an error if the proof or key is invalid.
func (*IAVLServer) VerifyItem(ctx context.Context, req *pb.VerifyItemRequest) (*empty.Empty, error) {
	proof := iavl.ConvertProtoRangeProof(req.Proof)
	if err := proof.Verify(req.RootHash); err != nil {
		return nil, err
	}

	if err := proof.VerifyItem(req.Key, req.Value); err != nil {
		return nil, err
	}

	return &empty.Empty{}, nil
}

// VerifyAbsence verifies the absence of a given key in an IAVL range proof
// returning an error if the proof or key is invalid.
func (*IAVLServer) VerifyAbsence(ctx context.Context, req *pb.VerifyAbsenceRequest) (*empty.Empty, error) {
	proof := iavl.ConvertProtoRangeProof(req.Proof)
	if err := proof.Verify(req.RootHash); err != nil {
		return nil, err
	}

	if err := proof.VerifyAbsence(req.Key); err != nil {
		return nil, err
	}

	return &empty.Empty{}, nil
}

// Rollback resets the working tree to the latest saved version, discarding
// any unsaved modifications.
func (s *IAVLServer) Rollback(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	s.tree.Rollback()
	return &empty.Empty{}, nil
}
