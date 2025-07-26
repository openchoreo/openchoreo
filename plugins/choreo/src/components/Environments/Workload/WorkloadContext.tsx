import React, { createContext, useContext } from 'react';
import { ModelsBuild, ModelsWorkload } from '@internal/plugin-openchoreo-api';

interface WorkloadContextType {
    builds: ModelsBuild[];
    workloadSpec: ModelsWorkload | null;
    setWorkloadSpec: (spec: ModelsWorkload | null) => void;
}

const WorkloadContext = createContext<WorkloadContextType | undefined>(undefined);

export const WorkloadProvider: React.FC<{ 
    builds: ModelsBuild[]; 
    workloadSpec: ModelsWorkload | null;
    setWorkloadSpec: (spec: ModelsWorkload | null) => void;
    children: React.ReactNode; 
}> = ({ builds, workloadSpec, setWorkloadSpec, children }) => {
    return (
        <WorkloadContext.Provider value={{ builds, workloadSpec, setWorkloadSpec }}>
            {children}
        </WorkloadContext.Provider>
    );
};

export const useWorkloadContext = (): WorkloadContextType => {
    const context = useContext(WorkloadContext);
    if (context === undefined) {
        throw new Error('useWorkloadContext must be used within a WorkloadProvider');
    }
    return context;
};

// Keep backwards compatibility
export const useBuilds = () => {
    const { builds } = useWorkloadContext();
    return { builds };
}; 